// Command ppdd_exporter is a Prometheus exporter for Dell PowerProtect DD appliances.
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os/signal"
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
	var once, debug bool
	root := &cobra.Command{
		Use:     "ppdd_exporter",
		Version: version,
		RunE: func(_ *cobra.Command, _ []string) error {
			return run(cfgPath, once, debug)
		},
	}
	root.Flags().StringVar(&cfgPath, "config", "config.yaml", "path to config file")
	root.Flags().BoolVar(&once, "once", false, "run a single collection cycle and exit")
	root.Flags().BoolVar(&debug, "debug", false, "verbose logging")
	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}

func run(cfgPath string, once, debug bool) error {
	if debug {
		log.SetLevel(log.DebugLevel)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	clients := make([]ddclient.Client, 0, len(cfg.Systems))
	for _, s := range cfg.Systems {
		clients = append(clients, ddclient.NewSystemClient(ddclient.Config{
			Name: s.Name, BaseURL: s.BaseURL(), Username: s.Username,
			Password: s.Password, InsecureSkipVerify: s.InsecureSkipVerify,
		}))
	}
	defer func() {
		for _, c := range clients {
			_ = c.Close()
		}
	}()

	store := ppdd.NewSnapshotStore()
	col := ppdd.NewCollector(clients, ppdd.Registry(), store, cfg.Collection.Interval, cfg.Collection.Timeout)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Info("running initial collection cycle")
	col.CollectOnce(ctx)
	if once {
		return nil
	}
	go col.Run(ctx)

	if w, err := config.NewWatcher(cfgPath); err == nil {
		defer w.Close()
		go func() {
			for newCfg := range w.Updates() {
				log.WithField("systems", len(newCfg.Systems)).
					Info("config reloaded (restart to apply system/client changes)")
			}
		}()
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(ppdd.NewPromCollector(store))

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
