package cli

import (
	"bytes"
	"testing"
	"time"

	"github.com/wavever/CCLimitPing/internal/config"
	"github.com/wavever/CCLimitPing/internal/usage"
)

// testWeeklyThreshold mirrors the config default (cfg.WeeklyThreshold = 0.99).
const testWeeklyThreshold = 0.99

func u(fiveHour, weekly float64, limitReached bool) *usage.Usage {
	return &usage.Usage{
		FiveHour:     usage.Window{UsedPercent: fiveHour},
		Weekly:       usage.Window{UsedPercent: weekly},
		LimitReached: limitReached,
	}
}

func newTestArmer() *continueArmer {
	return &continueArmer{weeklyThreshold: testWeeklyThreshold}
}

func TestContinueArmerFiresOnRecoveryEdge(t *testing.T) {
	a := newTestArmer()

	if a.observe(u(40, 10, false), false) {
		t.Fatal("should not fire while well under the limit")
	}
	if a.observe(u(99, 20, false), false) {
		t.Fatal("should not fire at the moment the limit is hit (only park)")
	}
	if !a.parked {
		t.Fatal("should be parked after the 5h window maxes out (>=95%)")
	}
	if !a.observe(u(3, 20, false), false) {
		t.Fatal("should fire once when the 5h window resets (drops below 50%)")
	}
	if a.parked {
		t.Fatal("should unpark after firing")
	}
	if a.observe(u(3, 20, false), false) {
		t.Fatal("should not fire again without re-parking")
	}
}

func TestContinueArmerWaitsForWeekly(t *testing.T) {
	a := newTestArmer()
	a.observe(u(100, 100, true), false) // parked, both windows exhausted
	if a.observe(u(3, 100, false), false) {
		t.Fatal("should not fire while the weekly window is still exhausted")
	}
	if !a.parked {
		t.Fatal("should stay parked until weekly recovers")
	}
	if !a.observe(u(3, 30, false), false) {
		t.Fatal("should fire once weekly recovers too")
	}
}

func TestContinueArmerWeeklyNearCapBlocksFire(t *testing.T) {
	a := newTestArmer()
	// The endpoint may report 99.x at the cap, never exactly 100 — the weekly
	// gate must treat that as exhausted (same threshold rule as the scheduler).
	a.observe(u(99, 99.5, false), false)
	if a.observe(u(3, 99.5, false), false) {
		t.Fatal("weekly at 99.x (>= threshold) must block the fire")
	}
	if !a.parked {
		t.Fatal("should stay parked while weekly is at the cap")
	}
	if !a.observe(u(3, 50, false), false) {
		t.Fatal("should fire once weekly drops below the threshold")
	}
}

func TestContinueArmerNoInjectLoopWhileLimitReached(t *testing.T) {
	a := newTestArmer()
	// Codex weekly cap: the API reports limit_reached while the 5h window is
	// low. This must park without firing — firing here would repeat every poll.
	if a.observe(u(30, 99.5, true), false) {
		t.Fatal("must not fire while limit_reached is true")
	}
	if !a.parked {
		t.Fatal("limit_reached should park the session")
	}
	if a.observe(u(30, 99.5, true), false) {
		t.Fatal("must not fire on repeat polls either")
	}
	if !a.observe(u(30, 20, false), false) {
		t.Fatal("should fire once the limit clears")
	}
}

func TestContinueArmerCreditsBypassWeekly(t *testing.T) {
	a := newTestArmer()
	a.observe(u(99, 100, false), false)
	rec := u(3, 100, false)
	rec.Credits = &usage.Credits{HasCredits: true}
	if !a.observe(rec, false) {
		t.Fatal("usable credits should allow continuing despite an exhausted weekly window")
	}
}

func TestContinueArmerParksViaLimitReached(t *testing.T) {
	a := newTestArmer()
	a.observe(u(80, 10, true), false) // limit_reached even though 5h < 95%
	if !a.parked {
		t.Fatal("limit_reached should park the session")
	}
	if !a.observe(u(3, 10, false), false) {
		t.Fatal("should fire when the window resets")
	}
}

func TestContinueArmerParksViaOutputWhenUsageCorroborates(t *testing.T) {
	a := newTestArmer()
	// On-screen limit message while the 5h window is still meaningfully used
	// (60% — endpoint lag) should park even though 5h hasn't reached 95%.
	if a.observe(u(60, 10, false), true) {
		t.Fatal("should park, not fire, while still used")
	}
	if !a.parked {
		t.Fatal("output limit message + used window should park")
	}
	if !a.observe(u(2, 10, false), false) {
		t.Fatal("should fire on reset")
	}
}

func TestContinueArmerIgnoresOutputWhenUsageLow(t *testing.T) {
	a := newTestArmer()
	// A stray "rate limit" in normal output while the window is nearly empty
	// must not park (and therefore must not spuriously fire).
	if a.observe(u(20, 10, false), true) {
		t.Fatal("should not fire from a low-usage output match")
	}
	if a.parked {
		t.Fatal("should not park when usage shows the window is nearly empty")
	}
}

func TestContainsLimitPhrase(t *testing.T) {
	// The real message, with ANSI styling interspersed, must still match.
	styled := []byte("\x1b[2m\x1b[38;5;244mYou've hit your \x1b[1msession limit\x1b[0m \x1b[2m· resets 5:50am\x1b[0m")
	if !containsLimitPhrase(styled) {
		t.Error("should match the styled 'session limit' message")
	}
	if !containsLimitPhrase([]byte("Error: rate limit exceeded")) {
		t.Error("should match 'rate limit'")
	}
	if containsLimitPhrase([]byte("the build finished, all tests green")) {
		t.Error("should not match unrelated output")
	}
}

func TestContainsClaudeLimitQuestion(t *testing.T) {
	styled := []byte("\x1b[1mAsk User Question\x1b[0m\nYou've hit your session limit.\n❯ Continue waiting until limit resets\n  Upgrade to Pro")
	if !containsClaudeLimitQuestion(styled) {
		t.Fatal("should match Claude's limit-specific Ask User Question")
	}
	if containsClaudeLimitQuestion([]byte("Ask User Question\nWhich file should I edit next?")) {
		t.Fatal("must not match ordinary task questions")
	}
}

func TestLimitDetectorRaisesSignalOnce(t *testing.T) {
	sig := &limitSignal{}
	d := &limitDetector{sig: sig}
	_, _ = d.Write([]byte("normal output\n"))
	if sig.seen() {
		t.Fatal("no limit message yet")
	}
	_, _ = d.Write([]byte("You've hit your session limit · resets 5:50am\n"))
	if !sig.seen() {
		t.Fatal("should raise the signal on the limit message")
	}
	sig.clear()
	if sig.seen() {
		t.Fatal("clear should reset the signal")
	}
}

func TestLimitDetectorResetDropsStaleBanner(t *testing.T) {
	d := &limitDetector{sig: &limitSignal{}}
	_, _ = d.Write([]byte("You've hit your session limit · resets 5:50am\n"))
	if !d.seen() {
		t.Fatal("banner should raise the signal")
	}
	d.reset()
	if d.seen() {
		t.Fatal("reset must clear the signal")
	}
	_, _ = d.Write([]byte("normal output after recovery\n"))
	if d.seen() {
		t.Fatal("stale banner in the old tail must not re-raise the signal after reset")
	}
}

func TestLimitDetectorThrottlesScans(t *testing.T) {
	sig := &limitSignal{}
	d := &limitDetector{sig: sig, scanEvery: time.Hour}
	_, _ = d.Write([]byte("normal output\n")) // first write scans and arms the throttle
	_, _ = d.Write([]byte("You've hit your session limit\n"))
	if sig.seen() {
		t.Fatal("a write inside the throttle window must not be scanned")
	}
}

func TestLimitSignalGoesStale(t *testing.T) {
	sig := &limitSignal{fresh: time.Millisecond}
	sig.set()
	if !sig.seen() {
		t.Fatal("freshly raised signal should be seen")
	}
	time.Sleep(5 * time.Millisecond)
	if sig.seen() {
		t.Fatal("signal must decay once the message hasn't been seen for a while")
	}
}

type stringChanWriter chan string

func (w stringChanWriter) Write(p []byte) (int, error) {
	w <- string(p)
	return len(p), nil
}

func TestLimitDetectorConfirmsClaudeLimitQuestionOnce(t *testing.T) {
	ch := make(chan string, 2)
	d := &limitDetector{
		sig: &limitSignal{},
		claudeQuestion: &claudeLimitQuestionConfirmer{
			inject:   &sessionInjector{w: stringChanWriter(ch)},
			settle:   time.Nanosecond,
			cooldown: time.Hour,
		},
	}
	question := []byte("Ask User Question\nYou've hit your session limit.\nContinue waiting until limit resets\nUpgrade to Pro\n")
	_, _ = d.Write(question)
	select {
	case got := <-ch:
		if got != "\r" {
			t.Fatalf("confirmation write = %q, want CR", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Claude question confirmation")
	}

	_, _ = d.Write(question)
	select {
	case got := <-ch:
		t.Fatalf("confirmed more than once; extra write %q", got)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestClaudeLimitQuestionConfirmerAllowsLaterConfirmation(t *testing.T) {
	ch := make(chan string, 2)
	c := &claudeLimitQuestionConfirmer{
		inject:   &sessionInjector{w: stringChanWriter(ch)},
		settle:   time.Nanosecond,
		cooldown: time.Nanosecond,
	}
	c.confirm()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first confirmation")
	}
	time.Sleep(time.Millisecond)
	c.confirm()
	select {
	case got := <-ch:
		if got != "\r" {
			t.Fatalf("second confirmation write = %q, want CR", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for second confirmation")
	}
}

func TestSessionInjectorWritesMessageThenEnter(t *testing.T) {
	var buf bytes.Buffer
	inj := &sessionInjector{w: &buf}
	start := time.Now()
	inj.typeAndSubmit("继续任务", proxyInjectSettle)
	if got := buf.String(); got != "继续任务\r" {
		t.Errorf("injected %q, want message + CR", got)
	}
	if time.Since(start) < proxyInjectSettle {
		t.Error("expected a settle pause between message and Enter")
	}
}

func TestSessionInjectorSerializesSubmit(t *testing.T) {
	var buf bytes.Buffer
	inj := &sessionInjector{w: &buf}
	done := make(chan struct{})
	inj.mu.Lock() // simulate typeAndSubmit holding the lock mid-message
	go func() {
		inj.submit()
		close(done)
	}()
	time.Sleep(5 * time.Millisecond)
	if buf.Len() != 0 {
		t.Fatal("submit must wait for the in-flight injection")
	}
	inj.mu.Unlock()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for submit after unlock")
	}
	if got := buf.String(); got != "\r" {
		t.Fatalf("submit wrote %q, want CR", got)
	}
}

func TestContinueMessageDefaults(t *testing.T) {
	cfg := config.Default()
	if got := continueMessage(cfg, "claude"); got != "continue" {
		t.Errorf("default claude continue message = %q", got)
	}
	cfg.Codex.ContinuePrompt = ""
	if got := continueMessage(cfg, "codex"); got != "continue" {
		t.Errorf("empty codex continue message should fall back to %q, got %q", "continue", got)
	}
	cfg.Claude.ContinuePrompt = "继续任务"
	if got := continueMessage(cfg, "claude"); got != "继续任务" {
		t.Errorf("custom claude continue message = %q", got)
	}
}

// A nil/zero logger must be a safe no-op so callers never nil-check.
func TestProxyLoggerNilSafe(t *testing.T) {
	var lg *proxyLogger
	lg.logf("should not panic %d", 1)
	lg.close()
	(&proxyLogger{}).logf("also fine")
}
