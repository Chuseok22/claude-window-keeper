package scheduler

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/wavever/CCLimitPing/internal/config"
	"github.com/wavever/CCLimitPing/internal/provider"
	"github.com/wavever/CCLimitPing/internal/usage"
)

type stubProvider struct {
	mu       sync.Mutex
	usage    *usage.Usage
	reads    int
	triggers int
}

func (p *stubProvider) Name() string { return "stub" }

func (p *stubProvider) ReadUsage(context.Context) (*usage.Usage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reads++
	return p.usage, nil
}

func (p *stubProvider) Trigger(context.Context, bool) (*provider.TriggerResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.triggers++
	return &provider.TriggerResult{Command: "stub trigger"}, nil
}

func (p *stubProvider) counts() (reads, triggers int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.reads, p.triggers
}

func testConfig() config.Config {
	cfg := config.Default()
	cfg.Notify = false
	cfg.ResetBuffer = config.Duration{}
	return cfg
}

func waitFor(t *testing.T, d time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}

func TestRunTargetSleepsWhileFiveHourWindowActive(t *testing.T) {
	p := &stubProvider{
		usage: &usage.Usage{
			FiveHour: usage.Window{
				UsedPercent: 25,
				ResetsAt:    time.Now().Add(time.Second),
			},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := New(testConfig(), []Target{{Provider: p}}, false, false, io.Discard)
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runTarget(ctx, Target{Provider: p})
	}()

	waitFor(t, 200*time.Millisecond, func() bool {
		reads, _ := p.counts()
		return reads == 1
	})
	time.Sleep(50 * time.Millisecond)
	reads, triggers := p.counts()
	if reads != 1 || triggers != 0 {
		t.Fatalf("active window should sleep without polling/triggering; reads=%d triggers=%d", reads, triggers)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("runTarget did not stop after cancellation")
	}
}

func TestRunTargetWeeklyOnlySleepsUntilWeeklyReset(t *testing.T) {
	p := &stubProvider{
		usage: &usage.Usage{
			// FiveHour left zero: the provider does not enforce a 5h window
			// (Codex since 2026-07-12). Only the weekly window is running.
			Weekly: usage.Window{
				UsedPercent:   24,
				ResetsAt:      time.Now().Add(time.Second),
				WindowSeconds: 604800,
			},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := New(testConfig(), []Target{{Provider: p}}, false, false, io.Discard)
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runTarget(ctx, Target{Provider: p})
	}()

	waitFor(t, 200*time.Millisecond, func() bool {
		reads, _ := p.counts()
		return reads == 1
	})
	time.Sleep(50 * time.Millisecond)
	reads, triggers := p.counts()
	if reads != 1 || triggers != 0 {
		t.Fatalf("weekly-only regime should sleep until the weekly reset without pinging; reads=%d triggers=%d", reads, triggers)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("runTarget did not stop after cancellation")
	}
}

func TestRunTargetDryRunSleepsAfterEstimatedPing(t *testing.T) {
	p := &stubProvider{
		usage: &usage.Usage{
			FiveHour: usage.Window{WindowSeconds: 1},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := New(testConfig(), []Target{{Provider: p}}, true, false, io.Discard)
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runTarget(ctx, Target{Provider: p})
	}()

	waitFor(t, 200*time.Millisecond, func() bool {
		_, triggers := p.counts()
		return triggers == 1
	})
	time.Sleep(50 * time.Millisecond)
	reads, triggers := p.counts()
	if reads != 1 || triggers != 1 {
		t.Fatalf("dry-run should sleep on the estimated window without an immediate second usage read; reads=%d triggers=%d", reads, triggers)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("runTarget did not stop after cancellation")
	}
}
