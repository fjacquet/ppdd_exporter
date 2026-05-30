package config

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/fsnotify/fsnotify"
	log "github.com/sirupsen/logrus"
)

// Watcher reloads and revalidates the config on SIGHUP or file change, emitting the
// new *Config on Updates(). A bad reload is logged and dropped (the running config stays).
type Watcher struct {
	path    string
	fsw     *fsnotify.Watcher
	sigs    chan os.Signal
	updates chan *Config
	done    chan struct{}
}

// NewWatcher starts watching path. Call Close to stop.
func NewWatcher(path string) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := fsw.Add(path); err != nil {
		_ = fsw.Close()
		return nil, err
	}
	w := &Watcher{
		path: path, fsw: fsw,
		sigs:    make(chan os.Signal, 1),
		updates: make(chan *Config, 1),
		done:    make(chan struct{}),
	}
	signal.Notify(w.sigs, syscall.SIGHUP)
	go w.loop()
	return w, nil
}

// Updates is the channel of successfully reloaded configs.
func (w *Watcher) Updates() <-chan *Config { return w.updates }

// Trigger forces a reload (used by tests and callers that want a manual refresh).
func (w *Watcher) Trigger() { w.reload() }

func (w *Watcher) loop() {
	for {
		select {
		case <-w.done:
			return
		case <-w.sigs:
			w.reload()
		case ev := <-w.fsw.Events:
			if ev.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				w.reload()
			}
		case err := <-w.fsw.Errors:
			log.WithError(err).Warn("config watch error")
		}
	}
}

func (w *Watcher) reload() {
	cfg, err := Load(w.path)
	if err != nil {
		log.WithError(err).Warn("config reload failed; keeping current config")
		return
	}
	select {
	case w.updates <- cfg:
	default: // drop if a pending update is unread
	}
}

// Close stops the watcher.
func (w *Watcher) Close() error {
	close(w.done)
	signal.Stop(w.sigs)
	return w.fsw.Close()
}
