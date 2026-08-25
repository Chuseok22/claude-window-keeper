package scheduler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Chuseok22/claude-window-keeper/internal/config"
	"github.com/Chuseok22/claude-window-keeper/internal/notify"
	"github.com/Chuseok22/claude-window-keeper/internal/provider"
	"github.com/Chuseok22/claude-window-keeper/internal/usage"
)

type stubProvider struct {
	mu       sync.Mutex
	usage    *usage.Usage
	readErr  error
	trigErr  error
	reads    int
	triggers int
}

func (p *stubProvider) Name() string { return "stub" }

func (p *stubProvider) ReadUsage(context.Context) (*usage.Usage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reads++
	if p.readErr != nil {
		return nil, p.readErr
	}
	return p.usage, nil
}

func (p *stubProvider) Trigger(context.Context, bool) (*provider.TriggerResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.triggers++
	if p.trigErr != nil {
		return nil, p.trigErr
	}
	return &provider.TriggerResult{Command: "stub trigger"}, nil
}

func (p *stubProvider) counts() (reads, triggers int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.reads, p.triggers
}

func testConfig() config.Config {
	cfg := config.Default()
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

// runStub starts runTarget for p in the background and returns a stop func
// that cancels it and waits for the loop to exit.
func runStub(t *testing.T, target Target) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	s := New(testConfig(), []Target{target}, false, false, io.Discard)
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runTarget(ctx, target)
	}()
	return func() {
		cancel()
		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
			t.Fatal("runTarget did not stop after cancellation")
		}
	}
}

// settleAndCount waits for the first usage read, gives the loop a beat to act,
// and returns the counters.
func settleAndCount(t *testing.T, p *stubProvider) (reads, triggers int) {
	t.Helper()
	waitFor(t, 500*time.Millisecond, func() bool {
		reads, _ := p.counts()
		return reads >= 1
	})
	time.Sleep(50 * time.Millisecond)
	return p.counts()
}

func TestRunTargetWeeklyExhaustedSleepsUntilReset(t *testing.T) {
	p := &stubProvider{
		usage: &usage.Usage{
			FiveHour: usage.Window{WindowSeconds: 18000},
			Weekly: usage.Window{
				UsedPercent:   100,
				ResetsAt:      time.Now().Add(time.Hour),
				WindowSeconds: 604800,
			},
		},
	}
	stop := runStub(t, Target{Provider: p})
	defer stop()

	reads, triggers := settleAndCount(t, p)
	if reads != 1 || triggers != 0 {
		t.Fatalf("exhausted weekly should sleep without pinging; reads=%d triggers=%d", reads, triggers)
	}
}

func TestRunTargetCreditsBypassWeeklyLimit(t *testing.T) {
	p := &stubProvider{
		usage: &usage.Usage{
			FiveHour: usage.Window{WindowSeconds: 18000},
			Weekly: usage.Window{
				UsedPercent:   100,
				ResetsAt:      time.Now().Add(time.Hour),
				WindowSeconds: 604800,
			},
			Credits: &usage.Credits{HasCredits: true},
		},
	}
	stop := runStub(t, Target{Provider: p})
	defer stop()

	waitFor(t, 500*time.Millisecond, func() bool {
		_, triggers := p.counts()
		return triggers == 1
	})
}

func TestRunTargetPausesReadsWhenUsageRateLimited(t *testing.T) {
	p := &stubProvider{
		readErr: &provider.UsageHTTPError{
			StatusCode: 429,
			RetryAfter: time.Now().Add(time.Hour),
		},
	}
	stop := runStub(t, Target{Provider: p})
	defer stop()

	reads, triggers := settleAndCount(t, p)
	if reads != 1 || triggers != 0 {
		t.Fatalf("429 should pause reads until Retry-After; reads=%d triggers=%d", reads, triggers)
	}
}

func TestRunTargetBacksOffOnReadError(t *testing.T) {
	p := &stubProvider{readErr: errors.New("connection reset")}
	stop := runStub(t, Target{Provider: p})
	defer stop()

	reads, triggers := settleAndCount(t, p)
	if reads != 1 || triggers != 0 {
		t.Fatalf("read error should back off without pinging; reads=%d triggers=%d", reads, triggers)
	}
}

func TestRunTargetHonorsAlignStart(t *testing.T) {
	p := &stubProvider{
		usage: &usage.Usage{FiveHour: usage.Window{WindowSeconds: 18000}},
	}
	// settleAndCount asserts at roughly t+60ms; the align gate is set far enough
	// out that scheduling jitter under `go test -race` can't reach it.
	align := time.Now().Add(time.Second)
	stop := runStub(t, Target{Provider: p, AlignStart: align})
	defer stop()

	reads, triggers := settleAndCount(t, p)
	if reads != 1 || triggers != 0 {
		t.Fatalf("should wait for align_start before pinging; reads=%d triggers=%d", reads, triggers)
	}
	waitFor(t, 3*time.Second, func() bool {
		_, triggers := p.counts()
		return triggers == 1
	})
	if time.Now().Before(align) {
		t.Fatal("pinged before align_start")
	}
}

func TestRunTargetTriggersWhenWindowFree(t *testing.T) {
	p := &stubProvider{
		usage: &usage.Usage{FiveHour: usage.Window{WindowSeconds: 18000}},
	}
	stop := runStub(t, Target{Provider: p})
	defer stop()

	waitFor(t, 500*time.Millisecond, func() bool {
		_, triggers := p.counts()
		return triggers == 1
	})
	// After a ping the loop waits postPingGrace before re-reading; no
	// immediate double ping.
	time.Sleep(50 * time.Millisecond)
	if _, triggers := p.counts(); triggers != 1 {
		t.Fatalf("triggers = %d, want exactly 1", triggers)
	}
}

func TestRunTargetBacksOffOnTriggerFailure(t *testing.T) {
	p := &stubProvider{
		usage:   &usage.Usage{FiveHour: usage.Window{WindowSeconds: 18000}},
		trigErr: errors.New("cli exploded"),
	}
	stop := runStub(t, Target{Provider: p})
	defer stop()

	waitFor(t, 500*time.Millisecond, func() bool {
		_, triggers := p.counts()
		return triggers == 1
	})
	time.Sleep(50 * time.Millisecond)
	if _, triggers := p.counts(); triggers != 1 {
		t.Fatalf("failed trigger should back off, not hammer; triggers = %d", triggers)
	}
}

func TestNextBackoff(t *testing.T) {
	if got := nextBackoff(minBackoff); got != time.Minute {
		t.Fatalf("nextBackoff(30s) = %v, want 1m", got)
	}
	if got := nextBackoff(maxBackoff); got != maxBackoff {
		t.Fatalf("nextBackoff at cap = %v, want %v", got, maxBackoff)
	}
	if got := nextBackoff(6 * time.Minute); got != maxBackoff {
		t.Fatalf("nextBackoff(6m) = %v, want capped at %v", got, maxBackoff)
	}
}

func TestUsageRateLimitWait(t *testing.T) {
	now := time.Now()
	if got := usageRateLimitWait(now.Add(2*time.Minute), now); got != 2*time.Minute {
		t.Fatalf("wait = %v, want the Retry-After delta", got)
	}
	if got := usageRateLimitWait(now.Add(-time.Minute), now); got != rateLimitPause {
		t.Fatalf("past Retry-After wait = %v, want default pause", got)
	}
	if got := usageRateLimitWait(time.Time{}, now); got != rateLimitPause {
		t.Fatalf("missing Retry-After wait = %v, want default pause", got)
	}
}

func TestWindowLen(t *testing.T) {
	if got := windowLen(usage.Window{WindowSeconds: 18000}); got != 5*time.Hour {
		t.Fatalf("windowLen = %v, want 5h", got)
	}
	if got := windowLen(usage.Window{}); got != defaultWindow {
		t.Fatalf("windowLen fallback = %v, want %v", got, defaultWindow)
	}
}

func TestTriggerCost(t *testing.T) {
	if got := triggerCost(nil); got != "" {
		t.Fatalf("triggerCost(nil) = %q", got)
	}
	if got := triggerCost(&provider.TriggerResult{}); got != "" {
		t.Fatalf("triggerCost(no usage) = %q", got)
	}
	res := &provider.TriggerResult{HasUsage: true, TotalTokens: 100, InputTokens: 90, OutputTokens: 10, CostUSD: 0.011}
	want := " — 100 tok (in 90 / out 10), $0.0110"
	if got := triggerCost(res); got != want {
		t.Fatalf("triggerCost = %q, want %q", got, want)
	}
}

// countingTransport is an http.RoundTripper that counts requests and answers
// them locally with a 200 OK, so tests can observe how many times
// notify.Notify actually attempted to send a Telegram message without any
// real network access and without needing a cross-package test seam into
// internal/notify (whose telegramBaseURL swap helper is unexported).
type countingTransport struct {
	mu    sync.Mutex
	count int
}

func (rt *countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	rt.mu.Lock()
	rt.count++
	rt.mu.Unlock()
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     make(http.Header),
	}, nil
}

func (rt *countingTransport) Count() int {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.count
}

// swapDefaultHTTPClientForTest points the process-wide http.DefaultClient
// (which notify.Notify sends through) at rt for the duration of the test.
func swapDefaultHTTPClientForTest(t *testing.T) *countingTransport {
	t.Helper()
	rt := &countingTransport{}
	orig := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: rt}
	t.Cleanup(func() { http.DefaultClient = orig })
	return rt
}

func TestNotifyAuthExpired_OncePerEpisode_ThenResetOnSuccess(t *testing.T) {
	rt := swapDefaultHTTPClientForTest(t)
	s := &Scheduler{
		notifyCfg:    notify.Config{BotToken: "t", ChatID: "c"},
		authNotified: make(map[string]bool),
	}
	authErr := &provider.AuthExpiredError{Err: errors.New("refresh rejected")}

	// Repeated failures within the same episode must notify exactly once.
	s.notifyAuthExpired("claude", authErr)
	s.notifyAuthExpired("claude", authErr)
	s.notifyAuthExpired("claude", authErr)
	if got := rt.Count(); got != 1 {
		t.Fatalf("notify calls during one episode = %d, want 1", got)
	}
	if !s.authNotified["claude"] {
		t.Fatal("authNotified should be true after a notified auth failure")
	}

	// A successful ReadUsage resets the flag — mirrors the
	// `s.authNotified[name] = false` line runTarget executes on its success
	// path.
	s.authNotified["claude"] = false

	// A new episode of failures must notify again, exactly once more.
	s.notifyAuthExpired("claude", authErr)
	s.notifyAuthExpired("claude", authErr)
	if got := rt.Count(); got != 2 {
		t.Fatalf("notify calls after reset = %d, want 2 (one more, for the new episode)", got)
	}
}

func TestRunTarget_AuthExpiredError_TriggersNotifyExactlyOnce(t *testing.T) {
	rt := swapDefaultHTTPClientForTest(t)
	p := &stubProvider{readErr: &provider.AuthExpiredError{Err: errors.New("refresh rejected")}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := New(testConfig(), []Target{{Provider: p}}, false, false, io.Discard)
	s.notifyCfg = notify.Config{BotToken: "t", ChatID: "c"} // enable notify; env is unset in test runs
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runTarget(ctx, s.targets[0])
	}()

	waitFor(t, 500*time.Millisecond, func() bool { return rt.Count() >= 1 })
	time.Sleep(50 * time.Millisecond)
	if got := rt.Count(); got != 1 {
		t.Fatalf("notify calls = %d, want exactly 1", got)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("runTarget did not stop after cancellation")
	}
}

func TestRunTarget_GenericReadError_DoesNotTriggerNotify(t *testing.T) {
	rt := swapDefaultHTTPClientForTest(t)
	p := &stubProvider{readErr: errors.New("connection reset")}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := New(testConfig(), []Target{{Provider: p}}, false, false, io.Discard)
	s.notifyCfg = notify.Config{BotToken: "t", ChatID: "c"} // enable notify; env is unset in test runs
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runTarget(ctx, s.targets[0])
	}()

	waitFor(t, 500*time.Millisecond, func() bool {
		reads, _ := p.counts()
		return reads >= 1
	})
	time.Sleep(50 * time.Millisecond)
	if got := rt.Count(); got != 0 {
		t.Fatalf("notify calls for a generic (non-auth-expired) read error = %d, want 0 — errors.As must not match AuthExpiredError", got)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("runTarget did not stop after cancellation")
	}
}
