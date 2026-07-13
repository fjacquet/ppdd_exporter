// Command ppdd_exporter is a Prometheus exporter for Dell PowerProtect DD appliances.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/signal"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fjacquet/ppdd_exporter/internal/config"
	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
	"github.com/fjacquet/ppdd_exporter/internal/ppdd"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	var cfgPath string
	var once, debug, trace bool
	root := &cobra.Command{
		Use:     "ppdd_exporter",
		Version: version,
		RunE: func(_ *cobra.Command, _ []string) error {
			return run(cfgPath, once, debug, trace)
		},
	}
	root.Flags().StringVar(&cfgPath, "config", "config.yaml", "path to config file")
	root.Flags().BoolVar(&once, "once", false, "run a single collection cycle and exit")
	root.Flags().BoolVar(&debug, "debug", false, "verbose logging")
	root.Flags().BoolVar(&trace, "trace", false, "log every DD API response body (live-appliance payload validation; very verbose)")
	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}

func run(cfgPath string, once, debug, trace bool) error {
	if debug {
		log.SetLevel(log.DebugLevel)
	}
	// Load .env (if present) before interpolation so the `cp .env.example .env`
	// quickstart works for bare-metal runs too; real env vars always win.
	config.LoadDotEnv(cfgPath)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

	store := ppdd.NewSnapshotStore()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if once {
		clients := buildClients(cfg, trace)
		col := ppdd.NewCollector(clients, ppdd.Registry(), store, cfg.Collection.Interval, cfg.Collection.Timeout)
		log.Info("running initial collection cycle")
		snap := col.CollectOnce(ctx)
		for _, c := range clients {
			_ = c.Close()
		}
		if debug {
			dumpSamples(snap)
		}
		for _, s := range snap.Systems {
			log.WithFields(log.Fields{"system": s.System, "ok": s.OK, "samples": len(s.Samples)}).
				Info("collection done")
		}
		return nil
	}

	// runner owns the live collection loop and its clients so config reloads can
	// rebuild and swap them in place. The SnapshotStore is shared and never
	// replaced, so /metrics and /health keep serving across a swap.
	runner := newCollectorRunner(store, trace)
	defer runner.stop()
	log.Info("running initial collection cycle")
	runner.apply(ctx, cfg)

	if w, err := config.NewWatcher(cfgPath); err == nil {
		defer func() { _ = w.Close() }()
		go func() {
			serverCfg := cfg.Server
			for {
				select {
				case <-ctx.Done():
					return
				case newCfg, ok := <-w.Updates():
					if !ok {
						return
					}
					runner.apply(ctx, newCfg)
					entry := log.WithField("systems", len(newCfg.Systems))
					if newCfg.Server != serverCfg {
						entry.Warn("config reloaded and applied; server host/port/uri changed — restart to apply those")
					} else {
						entry.Info("config reloaded and applied")
					}
				}
			}
		}()
	} else {
		log.WithError(err).Warn("config watcher disabled (failed to start)")
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(ppdd.NewPromCollector(store))
	reg.MustRegister(ppdd.NewBuildInfoCollector(version, runtime.Version()))

	mux := http.NewServeMux()
	mux.Handle(cfg.Server.URI, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		healthHandler(w, store)
	})

	srv := &http.Server{
		Addr:              cfg.Server.Host + ":" + cfg.Server.Port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	log.WithField("addr", srv.Addr).Info("serving metrics")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// collectorRunner owns the live collection loop and its DD clients so a config
// reload can rebuild and swap them atomically. apply() is serialized by the single
// watcher goroutine (plus the one startup call), so it needs no caller-side locking;
// the mutex only guards the swap against a concurrent stop() at shutdown.
type collectorRunner struct {
	store   *ppdd.SnapshotStore
	trace   bool
	mu      sync.Mutex
	clients []ddclient.Client
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func newCollectorRunner(store *ppdd.SnapshotStore, trace bool) *collectorRunner {
	return &collectorRunner{store: store, trace: trace}
}

// apply stops any running loop, then builds clients + a collector from cfg, runs one
// immediate cycle (so new systems appear without waiting a full interval), and starts
// the background loop.
func (r *collectorRunner) apply(parent context.Context, cfg *config.Config) {
	r.shutdownCurrent()

	clients := buildClients(cfg, r.trace)
	col := ppdd.NewCollector(clients, ppdd.Registry(), r.store, cfg.Collection.Interval, cfg.Collection.Timeout)
	loopCtx, cancel := context.WithCancel(parent)

	r.mu.Lock()
	r.clients, r.cancel = clients, cancel
	r.mu.Unlock()

	col.CollectOnce(loopCtx)
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		col.Run(loopCtx)
	}()
}

// shutdownCurrent cancels the running loop, waits for it to exit, and closes its
// clients. Safe to call when nothing is running.
func (r *collectorRunner) shutdownCurrent() {
	r.mu.Lock()
	cancel, clients := r.cancel, r.clients
	r.cancel, r.clients = nil, nil
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	r.wg.Wait()
	for _, c := range clients {
		_ = c.Close()
	}
}

func (r *collectorRunner) stop() { r.shutdownCurrent() }

// buildClients constructs one DD client per configured system.
func buildClients(cfg *config.Config, trace bool) []ddclient.Client {
	clients := make([]ddclient.Client, 0, len(cfg.Systems))
	for _, s := range cfg.Systems {
		clients = append(clients, ddclient.NewSystemClient(ddclient.Config{
			Name: s.Name, BaseURL: s.BaseURL(), Username: s.Username,
			Password: s.Password, InsecureSkipVerify: s.InsecureSkipVerify.Bool(),
			Trace: trace,
		}))
	}
	return clients
}

// dumpSamples prints every collected sample in Prometheus exposition style,
// sorted, so a `--once --debug` run against a live appliance can be diffed
// against docs/metrics.md to spot silently-absent metrics.
func dumpSamples(snap *ppdd.Snapshot) {
	var lines []string
	for _, ss := range snap.Systems {
		for _, s := range ss.Samples {
			parts := make([]string, 0, len(s.Labels))
			for _, l := range s.Labels {
				parts = append(parts, fmt.Sprintf("%s=%q", l.Key, l.Value))
			}
			lines = append(lines, fmt.Sprintf("%s{%s} %v", s.Name, strings.Join(parts, ","), s.Value))
		}
	}
	sort.Strings(lines)
	for _, l := range lines {
		fmt.Println(l)
	}
}

func healthHandler(w http.ResponseWriter, store *ppdd.SnapshotStore) {
	snap := store.Load()
	type sysHealth struct {
		System     string `json:"system"`
		OK         bool   `json:"ok"`
		LastScrape string `json:"last_scrape"`
		Err        string `json:"err,omitempty"`
	}
	out := struct {
		BuiltAt string      `json:"built_at"`
		Systems []sysHealth `json:"systems"`
	}{BuiltAt: snap.BuiltAt.Format(time.RFC3339)}
	healthy := len(snap.Systems) > 0
	for _, s := range snap.Systems {
		out.Systems = append(out.Systems, sysHealth{s.System, s.OK, s.LastScrape.Format(time.RFC3339), s.Err})
		if !s.OK {
			healthy = false
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if !healthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(w).Encode(out)
}
