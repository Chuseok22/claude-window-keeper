package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAcquireWatchLockPreventsSecondWatcher(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	release, err := acquireWatchLock("codex", false)
	if err != nil {
		t.Fatalf("acquireWatchLock first: %v", err)
	}
	defer release()

	if _, err := acquireWatchLock("claude", true); err == nil {
		t.Fatal("acquireWatchLock second succeeded, want already-running error")
	} else if !strings.Contains(err.Error(), "watch") {
		t.Fatalf("acquireWatchLock second error = %q, want watch context", err)
	}

	release()

	releaseAgain, err := acquireWatchLock("claude", true)
	if err != nil {
		t.Fatalf("acquireWatchLock after release: %v", err)
	}
	releaseAgain()
}

func TestAcquireWatchLockClearsStaleLock(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	lockDir := filepath.Join(dir, "claude-window-keeper")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	stale := watchLockState{
		PID:       999999,
		Provider:  "codex",
		StartedAt: time.Now().Add(-time.Hour),
	}
	data, err := json.Marshal(stale)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lockDir, watchLockName), data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	release, err := acquireWatchLock("claude", false)
	if err != nil {
		t.Fatalf("acquireWatchLock with stale lock: %v", err)
	}
	release()
}
