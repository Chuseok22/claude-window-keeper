package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Chuseok22/claude-window-keeper/internal/config"
)

const watchLockName = "watch.lock"

type watchLockState struct {
	PID       int       `json:"pid"`
	Provider  string    `json:"provider"`
	DryRun    bool      `json:"dry_run"`
	StartedAt time.Time `json:"started_at"`
}

// heldLocks tracks, per lock path, whether *this* process is the one that
// currently holds it. A lock file's PID field alone can't tell us that: in
// Docker the daemon always runs as PID 1, old containers are killed with
// SIGKILL (so the deferred release in acquireWatchLock never runs), and the
// volume holding config.Dir() persists across container recreations -- so a
// stale lock left by a *previous, now-dead* PID-1 process is
// indistinguishable, by PID alone, from a lock genuinely held by *this*
// process. heldLocks resolves that: it's only set when this process actually
// completes an acquisition, so a lock file naming our PID that we never
// acquired is recognized as a leftover from an earlier process, not a
// same-process double-acquire.
var (
	heldLocksMu sync.Mutex
	heldLocks   = make(map[string]bool)
)

func markLockHeld(path string) {
	heldLocksMu.Lock()
	heldLocks[path] = true
	heldLocksMu.Unlock()
}

func lockHeldByUs(path string) bool {
	heldLocksMu.Lock()
	defer heldLocksMu.Unlock()
	return heldLocks[path]
}

func clearLockHeld(path string) {
	heldLocksMu.Lock()
	delete(heldLocks, path)
	heldLocksMu.Unlock()
}

func acquireWatchLock(out io.Writer, provider string, dryRun bool) (func(), error) {
	logger := log.New(out, "", log.LstdFlags)
	dir, err := config.Dir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, watchLockName)

	for {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			st := watchLockState{
				PID:       os.Getpid(),
				Provider:  provider,
				DryRun:    dryRun,
				StartedAt: time.Now(),
			}
			data, jerr := json.MarshalIndent(st, "", "  ")
			if jerr == nil {
				_, jerr = f.Write(append(data, '\n'))
			}
			cerr := f.Close()
			if jerr != nil {
				_ = os.Remove(path)
				return nil, jerr
			}
			if cerr != nil {
				_ = os.Remove(path)
				return nil, cerr
			}
			markLockHeld(path)
			return func() { releaseWatchLock(path, st.PID) }, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}

		st, ok := readWatchLockPath(path)
		// A foreign, still-alive process holds the lock -- a real conflict.
		foreignAlive := ok && st.PID != os.Getpid() && processAlive(st.PID)
		// The lock file names our own PID *and* we're the process that
		// actually acquired it (still holding it, never released) -- also a
		// real conflict. If it names our PID but we never acquired it, it's a
		// stale leftover from an earlier, now-dead process that happened to
		// reuse this PID (see heldLocks doc comment above).
		selfHeld := ok && st.PID == os.Getpid() && lockHeldByUs(path)
		if foreignAlive || selfHeld {
			return nil, watchAlreadyRunningError(st)
		}
		if rerr := os.Remove(path); rerr != nil {
			logger.Printf("watch.lock 삭제 실패: %v", rerr)
		}
	}
}

func activeWatchLock() (watchLockState, bool) {
	path, err := watchLockPath()
	if err != nil {
		return watchLockState{}, false
	}
	st, ok := readWatchLockPath(path)
	if !ok {
		return watchLockState{}, false
	}
	if processAlive(st.PID) {
		return st, true
	}
	_ = os.Remove(path)
	return watchLockState{}, false
}

func watchLockPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, watchLockName), nil
}

func readWatchLockPath(path string) (watchLockState, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return watchLockState{}, false
	}
	var st watchLockState
	if err := json.Unmarshal(data, &st); err != nil || st.PID <= 0 {
		return watchLockState{}, false
	}
	return st, true
}

func releaseWatchLock(path string, pid int) {
	clearLockHeld(path)
	st, ok := readWatchLockPath(path)
	if ok && st.PID != pid {
		return
	}
	_ = os.Remove(path)
}

func watchAlreadyRunningError(st watchLockState) error {
	started := st.StartedAt.Format("2006-01-02 15:04:05")
	return fmt.Errorf(localizedText().watchAlreadyRunningFmt, st.PID, st.Provider, dryRunNote(st.DryRun), started)
}

// dryRunNote returns a human-readable suffix for the dry-run flag.
func dryRunNote(dryRun bool) string {
	if dryRun {
		return " (dry-run)"
	}
	return ""
}
