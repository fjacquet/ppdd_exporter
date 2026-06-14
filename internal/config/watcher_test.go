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
	write("9441")

	w, err := NewWatcher(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := w.Close(); err != nil {
			t.Errorf("Watcher.Close: %v", err)
		}
	})

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

// TestWatcherReloadsOnAtomicRename exercises the real fs-event path (not Trigger):
// updating the file via the write-temp-then-rename pattern replaces the inode, which
// only reloads if the watcher follows the parent directory.
func TestWatcherReloadsOnAtomicRename(t *testing.T) {
	t.Setenv("DD01_PASSWORD", "p")
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	cfgBytes := func(port string) []byte {
		return []byte("server: {port: \"" + port + "\"}\ncollection: {interval: 5m}\n" +
			"systems:\n  - {name: dd01, host: h, username: u, password: \"${DD01_PASSWORD}\"}\n")
	}
	if err := os.WriteFile(path, cfgBytes("9441"), 0o600); err != nil {
		t.Fatal(err)
	}

	w, err := NewWatcher(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := w.Close(); err != nil {
			t.Errorf("Watcher.Close: %v", err)
		}
	})

	tmp := filepath.Join(dir, ".c.yaml.tmp")
	if err := os.WriteFile(tmp, cfgBytes("9100"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}

	select {
	case cfg := <-w.Updates():
		if cfg.Server.Port != "9100" {
			t.Fatalf("reloaded port = %s, want 9100", cfg.Server.Port)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no config update received after atomic rename")
	}
}
