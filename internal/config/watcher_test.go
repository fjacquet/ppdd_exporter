package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcherEmitsOnSIGHUPFunc(t *testing.T) {
	t.Setenv("DD01_PASSWORD", "p")
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	write := func(port string) {
		_ = os.WriteFile(path, []byte(
			"server: {port: \""+port+"\"}\ncollection: {interval: 5m}\n"+
				"systems:\n  - {name: dd01, host: h, username: u, password: \"${DD01_PASSWORD}\"}\n"), 0o600)
	}
	write("9099")

	w, err := NewWatcher(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	write("9100")
	w.Trigger() // simulate SIGHUP without sending a real signal

	select {
	case cfg := <-w.Updates():
		if cfg.Server.Port != "9100" {
			t.Fatalf("reloaded port = %s, want 9100", cfg.Server.Port)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no config update received")
	}
}
