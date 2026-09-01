package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAcquireWatchLockPreventsSecondWatcher(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	release, err := acquireWatchLock(io.Discard, "codex", false)
	if err != nil {
		t.Fatalf("acquireWatchLock first: %v", err)
	}
	defer release()

	if _, err := acquireWatchLock(io.Discard, "claude", true); err == nil {
		t.Fatal("acquireWatchLock second succeeded, want already-running error")
	} else if !strings.Contains(err.Error(), "watch") {
		t.Fatalf("acquireWatchLock second error = %q, want watch context", err)
	}

	release()

	releaseAgain, err := acquireWatchLock(io.Discard, "claude", true)
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

	release, err := acquireWatchLock(io.Discard, "claude", false)
	if err != nil {
		t.Fatalf("acquireWatchLock with stale lock: %v", err)
	}
	release()
}

// TestAcquireWatchLockTreatsOwnPIDAsStale reproduces the Docker redeploy
// scenario: the daemon always runs as PID 1 in the container, old containers
// are killed with SIGKILL (so the lock's deferred release never runs), and
// the volume holding config.Dir() persists across container recreations. The
// second container start then finds a lock file naming its own current PID --
// which must be treated as stale (it can't already hold a lock it hasn't
// acquired yet), not as a real conflict with processAlive trivially true.
func TestAcquireWatchLockTreatsOwnPIDAsStale(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	lockDir := filepath.Join(dir, "claude-window-keeper")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	selfStale := watchLockState{
		PID:       os.Getpid(),
		Provider:  "codex",
		StartedAt: time.Now().Add(-time.Hour),
	}
	data, err := json.Marshal(selfStale)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lockDir, watchLockName), data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	release, err := acquireWatchLock(io.Discard, "claude", false)
	if err != nil {
		t.Fatalf("acquireWatchLock with a lock naming our own PID: %v", err)
	}
	release()
}

// TestAcquireWatchLockGivesUpAfterRepeatedRemoveFailures reproduces the NAS
// scenario from GitHub issue #12: the watch.lock file's parent directory is
// not writable by this process (a UID/volume-ownership mismatch), so the
// stale-lock removal inside acquireWatchLock's loop keeps failing. Before
// the fix, this looped forever with no backoff and no log output. After the
// fix, it must give up after exactly maxLockRemoveRetries failed removal
// attempts and return a descriptive error, not hang.
func TestAcquireWatchLockGivesUpAfterRepeatedRemoveFailures(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: unix permission checks (and thus os.Remove failures) are bypassed")
	}

	origBackoff, origMax := lockRemoveRetryBackoff, maxLockRemoveRetries
	lockRemoveRetryBackoff = time.Millisecond
	maxLockRemoveRetries = 3
	t.Cleanup(func() {
		lockRemoveRetryBackoff, maxLockRemoveRetries = origBackoff, origMax
	})

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
	lockPath := filepath.Join(lockDir, watchLockName)
	if err := os.WriteFile(lockPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Deletion permission comes from the parent directory's write bit, not
	// the file's own mode -- so make the directory read+execute only.
	if err := os.Chmod(lockDir, 0o555); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(lockDir, 0o755) })

	var buf bytes.Buffer
	done := make(chan struct{})
	var release func()
	var acquireErr error
	go func() {
		release, acquireErr = acquireWatchLock(&buf, "claude", false)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("acquireWatchLock did not return within 5s -- looks like an unbounded retry loop")
	}

	if release != nil {
		release()
	}
	if acquireErr == nil {
		t.Fatal("acquireWatchLock succeeded, want an error after repeated removal failures")
	}
	if !strings.Contains(acquireErr.Error(), lockPath) {
		t.Fatalf("acquireWatchLock error = %q, want it to mention the lock path %q", acquireErr.Error(), lockPath)
	}
	// The implementation logs exactly one "삭제 실패" line per failed
	// removal attempt, so this should equal maxLockRemoveRetries exactly —
	// not merely "at least", and not counting substring "watch.lock" (which
	// appears twice per line: once in the log prefix, once inside the
	// wrapped *PathError's own message).
	if got := buf.String(); strings.Count(got, "삭제 실패") != maxLockRemoveRetries {
		t.Fatalf("expected exactly %d logged removal-failure lines, got %d in log output:\n%s", maxLockRemoveRetries, strings.Count(got, "삭제 실패"), got)
	}
}
