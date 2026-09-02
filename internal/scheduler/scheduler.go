// Package scheduler runs the watch loop: for each provider it sleeps until the
// 5h window resets, then triggers a minimal ping to start the next window,
// keeping windows back-to-back. It respects the weekly limit and never lets a
// transient error kill the loop. When requested on an interactive terminal, it
// also draws a live status line (heartbeat + per-provider countdowns) beneath
// the scrolling log.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/Chuseok22/claude-window-keeper/internal/config"
	"github.com/Chuseok22/claude-window-keeper/internal/notify"
	"github.com/Chuseok22/claude-window-keeper/internal/provider"
	"github.com/Chuseok22/claude-window-keeper/internal/usage"
)

const (
	postPingGrace     = 15 * time.Second // wait after a ping before re-reading usage
	minBackoff        = 30 * time.Second
	maxBackoff        = 10 * time.Minute
	rateLimitPause    = 5 * time.Minute
	defaultWindow     = 5 * time.Hour // fallback when the API omits the window length
	readTimeout       = 30 * time.Second
	triggerTimeout    = 3 * time.Minute
	maxVerifyFailures = 3 // consecutive Trigger()-verification failures before giving up and waiting for the next natural window cycle
)

// Target pairs a provider with its scheduling options.
type Target struct {
	Provider   provider.Provider
	AlignStart time.Time // zero = ping as soon as the window is free
	AutoRedeem bool      // spend a reset credit that is about to lapse
}

// Scheduler drives the watch loops.
type Scheduler struct {
	cfg           config.Config
	targets       []Target
	dryRun        bool
	log           *log.Logger
	live          *liveStatus
	notifyCfg     notify.Config
	notifySuccess bool       // whether a verified trigger success also sends a Discord alert
	authMu        sync.Mutex // guards authNotified, written from each target's goroutine
	authNotified  map[string]bool
}

// New builds a scheduler that logs to out. When live is true and out is an
// interactive terminal, a live status line is drawn beneath the scrolling log;
// otherwise log output passes straight through.
func New(cfg config.Config, targets []Target, dryRun, live bool, out io.Writer) *Scheduler {
	names := make([]string, len(targets))
	for i, t := range targets {
		names[i] = t.Provider.Name()
	}
	status := newLiveStatus(out, names, live)
	return &Scheduler{
		cfg:           cfg,
		targets:       targets,
		dryRun:        dryRun,
		log:           log.New(status, "", log.LstdFlags),
		live:          status,
		notifyCfg:     notify.Config{WebhookURL: os.Getenv("DISCORD_WEBHOOK_URL")},
		notifySuccess: envBoolDefaultTrue("DISCORD_NOTIFY_ON_SUCCESS"),
		authNotified:  make(map[string]bool),
	}
}

// Run starts one loop per target and blocks until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	names := make([]string, len(s.targets))
	for i, t := range s.targets {
		names[i] = t.Provider.Name()
	}
	s.log.Printf("watching %v (weekly_threshold=%.2f, reset_buffer=%s, dry_run=%t)",
		names, s.cfg.WeeklyThreshold, s.cfg.ResetBuffer.Duration, s.dryRun)
	if s.notifyCfg.Enabled() {
		s.log.Printf("discord alerting: enabled")
		if s.notifySuccess {
			s.log.Printf("discord success notify: enabled")
		} else {
			s.log.Printf("discord success notify: disabled (DISCORD_NOTIFY_ON_SUCCESS=false)")
		}
	} else {
		s.log.Printf("discord alerting: disabled (DISCORD_WEBHOOK_URL not set)")
	}

	var liveWG sync.WaitGroup
	if s.live.enabled {
		liveWG.Add(1)
		go func() {
			defer liveWG.Done()
			s.live.run(ctx)
		}()
	}

	done := make(chan struct{}, len(s.targets))
	for _, t := range s.targets {
		go func(t Target) {
			s.runTarget(ctx, t)
			done <- struct{}{}
		}(t)
	}
	for range s.targets {
		<-done
	}
	liveWG.Wait() // let the render loop clear its line before the final log
	s.log.Printf("shutting down")
}

func (s *Scheduler) runTarget(ctx context.Context, t Target) {
	name := t.Provider.Name()
	backoff := minBackoff
	verifyBackoff := minBackoff      // escalates independently on Trigger()-verification failures
	verifyFailures := 0              // consecutive verification failures; capped at maxVerifyFailures
	aligned := t.AlignStart.IsZero() // whether the align gate has been passed
	var lastPingAt time.Time

	for {
		if ctx.Err() != nil {
			return
		}

		s.live.set(name, "checking usage…", time.Time{})
		rctx, cancel := context.WithTimeout(ctx, readTimeout)
		u, err := t.Provider.ReadUsage(rctx)
		cancel()
		if err != nil {
			var httpErr *provider.UsageHTTPError
			if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusTooManyRequests {
				wait := usageRateLimitWait(httpErr.RetryAfter, time.Now())
				s.log.Printf("[%s] usage endpoint rate limited; pausing reads for %s", name, wait.Round(time.Second))
				s.live.set(name, "usage rate limited", time.Now().Add(wait))
				if !sleepCtx(ctx, wait) {
					return
				}
				backoff = minBackoff
				continue
			}
			var authErr *provider.AuthExpiredError
			if errors.As(err, &authErr) {
				s.notifyAuthExpired(name, authErr)
			}
			s.log.Printf("[%s] read usage failed: %v (retry in %s)", name, err, backoff)
			s.live.set(name, "read failed — retrying", time.Now().Add(backoff))
			if !sleepCtx(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}
		backoff = minBackoff
		s.setAuthNotified(name, false)

		// A spent credit resets the windows, so this snapshot is stale; re-read
		// before deciding anything from it.
		if s.redeemExpiringCredit(ctx, t, u) {
			continue
		}

		// Respect the weekly limit: if exhausted (and no usable credits), wait
		// for the weekly window to reset instead of pinging.
		if s.weeklyExhausted(u) {
			wait := u.Weekly.Remaining() + s.cfg.ResetBuffer.Duration
			if wait <= 0 {
				wait = time.Minute
			}
			s.log.Printf("[%s] weekly limit exhausted (%.0f%%); sleeping %s until weekly reset",
				name, u.Weekly.UsedPercent, wait.Round(time.Second))
			s.live.set(name, fmt.Sprintf("weekly limit reached (%.0f%%)", u.Weekly.UsedPercent), time.Now().Add(wait))
			if !sleepCtx(ctx, wait) {
				return
			}
			continue
		}

		// A provider can stop enforcing the 5h window entirely (OpenAI
		// temporarily removed Codex's on 2026-07-12, leaving only the weekly
		// cap). The running weekly window is already anchored, so the next
		// useful ping is the one that anchors the weekly window right after
		// its reset — pinging every 5h would just burn weekly quota.
		if u.FiveHour.Missing() && u.Weekly.Active() {
			wait := u.Weekly.Remaining() + s.cfg.ResetBuffer.Duration
			s.log.Printf("[%s] no 5h window (weekly-only limits, %.0f%%); next ping at weekly reset %s (in %s)",
				name, u.Weekly.UsedPercent,
				u.Weekly.ResetsAt.Local().Format("15:04:05"), wait.Round(time.Second))
			s.live.set(name, fmt.Sprintf("weekly-only %.0f%% — ping at weekly reset", u.Weekly.UsedPercent), time.Now().Add(wait))
			if !sleepCtx(ctx, wait) {
				return
			}
			continue
		}

		// If the 5h window is still running, wait until it resets, then ping.
		if u.FiveHour.Active() {
			// An active window is proof the previous ping (if any) actually
			// landed, even if an earlier verification read caught it too
			// early. Reset the verify-failure streak so a stale count from
			// a prior episode can't trip the cap on this episode's first
			// failure.
			verifyFailures = 0
			verifyBackoff = minBackoff
			wait := u.FiveHour.Remaining() + s.cfg.ResetBuffer.Duration
			s.log.Printf("[%s] 5h window active (%.0f%%), next ping at %s (in %s)",
				name, u.FiveHour.UsedPercent,
				u.FiveHour.ResetsAt.Local().Format("15:04:05"), wait.Round(time.Second))
			s.live.set(name, fmt.Sprintf("5h window %.0f%% — next ping", u.FiveHour.UsedPercent), time.Now().Add(wait))
			if !sleepCtx(ctx, wait) {
				return
			}
			continue
		}

		// Window is free. Guard against double-pinging if our last ping isn't
		// reflected by the endpoint yet.
		if !lastPingAt.IsZero() {
			est := lastPingAt.Add(windowLen(u.FiveHour))
			if time.Now().Before(est) {
				wait := time.Until(est) + s.cfg.ResetBuffer.Duration
				s.log.Printf("[%s] recent ping not yet visible; waiting %s", name, wait.Round(time.Second))
				s.live.set(name, "awaiting window", time.Now().Add(wait))
				if !sleepCtx(ctx, wait) {
					return
				}
				continue
			}
		}

		// Honor the first-window alignment anchor, once.
		if !aligned {
			if d := time.Until(t.AlignStart); d > 0 {
				s.log.Printf("[%s] waiting for align_start %s (in %s)",
					name, t.AlignStart.Local().Format("15:04:05"), d.Round(time.Second))
				s.live.set(name, "waiting for align_start", t.AlignStart)
				if !sleepCtx(ctx, d) {
					return
				}
			}
			aligned = true
		}

		// Trigger the window.
		if !s.dryRun {
			s.log.Printf("[%s] window reset — triggering ping now…", name)
		}
		s.live.set(name, "window reset — triggering ping…", time.Time{})
		tctx, tcancel := context.WithTimeout(ctx, triggerTimeout)
		res, err := t.Provider.Trigger(tctx, s.dryRun)
		tcancel()
		if s.dryRun {
			if err != nil {
				s.log.Printf("[%s] dry-run ping failed: %v (retry in %s)", name, err, backoff)
				if !sleepCtx(ctx, backoff) {
					return
				}
				backoff = nextBackoff(backoff)
				continue
			}
			s.log.Printf("[%s] DRY-RUN would ping now: %s", name, res.Command)
			// In dry-run we can't actually start a window, so estimate the next
			// cycle from the configured window length to keep the loop sane.
			// Sleep immediately instead of doing an extra usage read that cannot
			// observe a real newly-started window.
			lastPingAt = time.Now()
			wait := windowLen(u.FiveHour) + s.cfg.ResetBuffer.Duration
			s.live.set(name, "dry-run — next estimated ping", lastPingAt.Add(wait))
			if !sleepCtx(ctx, wait) {
				return
			}
			continue
		}
		if err != nil {
			s.log.Printf("[%s] ping failed: %v (retry in %s)", name, err, backoff)
			s.live.set(name, "ping failed — retrying", time.Now().Add(backoff))
			if !sleepCtx(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}
		lastPingAt = time.Now()
		s.log.Printf("[%s] ping sent, verifying window started%s", name, triggerCost(res))
		s.live.set(name, "ping sent — verifying window…", lastPingAt.Add(postPingGrace))

		if !sleepCtx(ctx, postPingGrace) {
			return
		}

		vctx, vcancel := context.WithTimeout(ctx, readTimeout)
		vu, verr := t.Provider.ReadUsage(vctx)
		vcancel()
		if verr != nil || !vu.FiveHour.Active() {
			verifyFailures++
			wait := verifyBackoff
			capped := verifyFailures >= maxVerifyFailures
			switch {
			case capped:
				// Trigger() keeps claiming success without ever opening a
				// window (e.g. stuck behind a login prompt). Stop hammering
				// it: leave lastPingAt as-is so the duplicate-ping guard
				// above enforces a full natural-window wait before the next
				// attempt, instead of retrying every backoff cycle forever.
				s.log.Printf("[%s] %d consecutive verification failures — pausing retries until the next natural window cycle", name, verifyFailures)
			case verr != nil:
				s.log.Printf("[%s] ping verification read failed: %v (retry in %s)", name, verr, wait)
			default:
				s.log.Printf("[%s] ping verification failed: window not active after ping (retry in %s)", name, wait)
			}
			if capped {
				verifyFailures = 0
				verifyBackoff = minBackoff
				s.live.set(name, "ping unverified — pausing for natural cycle", time.Time{})
			} else {
				lastPingAt = time.Time{} // unverified — don't block the next attempt for a full window
				verifyBackoff = nextBackoff(verifyBackoff)
				s.live.set(name, "ping unverified — retrying", time.Now().Add(wait))
			}
			if !sleepCtx(ctx, wait) {
				return
			}
			continue
		}
		verifyFailures = 0
		verifyBackoff = minBackoff
		backoff = minBackoff
		s.log.Printf("[%s] window verified active", name)
		s.notifyTriggerSucceeded(name, vu.FiveHour, res)
	}
}

func (s *Scheduler) weeklyExhausted(u *usage.Usage) bool {
	return u.WeeklyExhausted(s.cfg.WeeklyThreshold)
}

// redeemExpiringCredit spends a banked reset credit that is about to lapse,
// when the target opted in. It reports whether the windows were actually reset;
// a failure is logged and never breaks the loop.
func (s *Scheduler) redeemExpiringCredit(ctx context.Context, t Target, u *usage.Usage) bool {
	redeemer, ok := t.Provider.(provider.ResetCreditRedeemer)
	if !t.AutoRedeem || !ok || s.dryRun {
		return false
	}
	name := t.Provider.Name()
	rctx, cancel := context.WithTimeout(ctx, readTimeout)
	outcome, err := redeemer.AutoRedeemResetCredit(rctx, u)
	cancel()
	switch {
	case err != nil:
		s.log.Printf("[%s] reset credit redeem failed: %v", name, err)
	case outcome == provider.RedeemReset:
		s.log.Printf("[%s] redeemed an expiring reset credit; rate-limit windows reset", name)
		return true
	case outcome != "":
		s.log.Printf("[%s] reset credit not spent: %s", name, outcome)
	}
	return false
}

// notifyAuthExpired sends a Discord alert the first time name's refresh
// token is seen to be definitively rejected, then stays silent until a
// successful ReadUsage resets the flag (see runTarget). A failed send is
// logged, not retried — a missed alert must never block the watch loop.
func (s *Scheduler) notifyAuthExpired(name string, err error) {
	if s.authWasNotified(name) {
		return
	}
	s.setAuthNotified(name, true)
	if nerr := notify.Notify(s.notifyCfg, name+": 인증이 만료됐습니다 — 다시 로그인해 주세요",
		"자격증명 파일을 갱신할 때까지 재시도만 계속합니다.\n"+err.Error()); nerr != nil {
		s.log.Printf("[%s] discord 알림 전송 실패: %v", name, nerr)
	}
}

// notifyTriggerSucceeded sends a Discord alert confirming a provider's 5h
// window was verified active after a trigger. Gated by notifySuccess
// (DISCORD_NOTIFY_ON_SUCCESS, default true — see New()). Like
// notifyAuthExpired, a failed send is logged and never retried; it must
// never block the watch loop.
func (s *Scheduler) notifyTriggerSucceeded(name string, w usage.Window, res *provider.TriggerResult) {
	if !s.notifySuccess {
		return
	}
	msg := fmt.Sprintf("다음 리셋 예정: %s%s", w.ResetsAt.Local().Format("15:04:05"), triggerCost(res))
	if nerr := notify.Notify(s.notifyCfg, name+": 5시간 세션이 시작됐습니다", msg); nerr != nil {
		s.log.Printf("[%s] discord 성공 알림 전송 실패: %v", name, nerr)
	}
}

// authWasNotified reports whether name's auth-expired alert has already been
// sent for the current episode. Safe for concurrent use: Run launches one
// goroutine per target, and every goroutine shares this map.
func (s *Scheduler) authWasNotified(name string) bool {
	s.authMu.Lock()
	defer s.authMu.Unlock()
	return s.authNotified[name]
}

// setAuthNotified records whether name's auth-expired alert has been sent.
// Safe for concurrent use (see authWasNotified).
func (s *Scheduler) setAuthNotified(name string, v bool) {
	s.authMu.Lock()
	defer s.authMu.Unlock()
	s.authNotified[name] = v
}

// triggerCost renders the token/cost tail for logs, e.g.
// " — 32934 tok (in 32792 / out 142), $0.0110".
func triggerCost(res *provider.TriggerResult) string {
	if res == nil || !res.HasUsage {
		return ""
	}
	s := fmt.Sprintf(" — %d tok (in %d / out %d)", res.TotalTokens, res.InputTokens, res.OutputTokens)
	if res.CostUSD > 0 {
		s += fmt.Sprintf(", $%.4f", res.CostUSD)
	}
	return s
}

func windowLen(w usage.Window) time.Duration {
	if w.WindowSeconds > 0 {
		return time.Duration(w.WindowSeconds) * time.Second
	}
	return defaultWindow
}

func nextBackoff(d time.Duration) time.Duration {
	d *= 2
	if d > maxBackoff {
		return maxBackoff
	}
	return d
}

func usageRateLimitWait(retryAfter time.Time, now time.Time) time.Duration {
	if !retryAfter.IsZero() {
		wait := retryAfter.Sub(now)
		if wait > 0 {
			return wait
		}
	}
	return rateLimitPause
}

// sleepCtx sleeps for d and reports false if the context was cancelled.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// envBoolDefaultTrue reads a boolean environment variable that defaults to
// true when unset or unparseable — used for DISCORD_NOTIFY_ON_SUCCESS, which
// should require an explicit opt-out rather than an explicit opt-in.
func envBoolDefaultTrue(key string) bool {
	v := os.Getenv(key)
	if v == "" {
		return true
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return true
	}
	return b
}
