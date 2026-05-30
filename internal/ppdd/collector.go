package ppdd

import (
	"context"
	"fmt"
	"time"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// Collector runs the background loop: every interval it polls all systems in
// parallel and publishes a fresh Snapshot. One system's failure never blocks others.
type Collector struct {
	clients    []ddclient.Client
	collectors []ResourceCollector
	store      *SnapshotStore
	interval   time.Duration
	timeout    time.Duration
}

// NewCollector wires the loop.
func NewCollector(clients []ddclient.Client, collectors []ResourceCollector, store *SnapshotStore, interval, timeout time.Duration) *Collector {
	return &Collector{clients: clients, collectors: collectors, store: store, interval: interval, timeout: timeout}
}

// CollectOnce runs a single cycle, stores, and returns the snapshot.
func (c *Collector) CollectOnce(ctx context.Context) *Snapshot {
	snap := c.collectAll(ctx)
	c.store.Store(snap)
	return snap
}

// Run loops until ctx is cancelled (assumes CollectOnce already primed the store).
func (c *Collector) Run(ctx context.Context) {
	t := time.NewTicker(c.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.store.Store(c.collectAll(ctx))
		}
	}
}

func (c *Collector) collectAll(ctx context.Context) *Snapshot {
	results := make([]*SystemSnapshot, len(c.clients))
	g, gctx := errgroup.WithContext(ctx)
	for i, client := range c.clients {
		i, client := i, client
		g.Go(func() error {
			results[i] = c.collectSystem(gctx, client)
			return nil // graceful degradation
		})
	}
	_ = g.Wait()
	return &Snapshot{BuiltAt: time.Now(), Systems: results}
}

func (c *Collector) collectSystem(ctx context.Context, client ddclient.Client) *SystemSnapshot {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	ss := &SystemSnapshot{System: client.Name(), LastScrape: time.Now(), OK: true}
	failures := 0
	var lastErr error
	domainSamples := 0
	for _, rc := range c.collectors {
		samples, err := rc.Collect(ctx, client)
		up := 1.0
		if err != nil {
			up = 0
			failures++
			lastErr = err
			log.WithFields(log.Fields{"system": client.Name(), "collector": rc.Name(), "err": err}).
				Warn("collector failed")
		}
		ss.Samples = append(ss.Samples, Sample{
			Name:   "ppdd_collector_up",
			Labels: []Label{{Key: "collector", Value: rc.Name()}},
			Value:  up,
		}.WithSystem(client.Name()))
		for _, s := range samples {
			ss.Samples = append(ss.Samples, s.WithSystem(client.Name()))
			domainSamples++
		}
	}
	switch {
	case len(c.collectors) > 0 && failures == len(c.collectors):
		ss.OK = false
		ss.Err = fmt.Sprintf("all %d collectors failed: %v", len(c.collectors), lastErr)
	case len(c.collectors) > 0 && domainSamples == 0:
		ss.OK = false
		ss.Err = fmt.Sprintf("no domain samples collected (failures: %d/%d)", failures, len(c.collectors))
	}
	return ss
}
