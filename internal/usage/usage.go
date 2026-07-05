// Package usage defines the normalized rate-limit usage model shared across
// providers. Readers translate each provider's raw API response into these
// types so the scheduler and CLI can treat every provider uniformly.
package usage

import "time"

// Window is a single rate-limit window (e.g. the 5h rolling window or the
// weekly window). UsedPercent is 0..100.
type Window struct {
	UsedPercent   float64
	ResetsAt      time.Time
	WindowSeconds int
}

// Active reports whether the window currently has consumption recorded and has
// not yet reset. A freshly reset (or never-started) window is inactive, which
// is the signal the scheduler uses to decide whether to ping immediately.
func (w Window) Active() bool {
	return w.UsedPercent > 0 && !w.ResetsAt.IsZero() && time.Now().Before(w.ResetsAt)
}

// Remaining returns the time until this window resets (never negative).
func (w Window) Remaining() time.Duration {
	if w.ResetsAt.IsZero() {
		return 0
	}
	d := time.Until(w.ResetsAt)
	if d < 0 {
		return 0
	}
	return d
}

// Credits describes pay-as-you-go credits that may remain available even when
// the weekly window is exhausted.
type Credits struct {
	HasCredits bool
	Unlimited  bool
	Balance    string
}

// ResetCredit is a banked Codex rate-limit reset credit.
type ResetCredit struct {
	Status     string
	GrantedAt  time.Time
	ExpiresAt  time.Time
	RedeemedAt time.Time
}

// ResetCredits summarizes account-backed Codex reset credits.
type ResetCredits struct {
	AvailableCount int
	Credits        []ResetCredit
}

// Usage is a provider's full rate-limit snapshot at FetchedAt.
type Usage struct {
	Provider     string
	FiveHour     Window
	Weekly       Window
	Plan         string
	Credits      *Credits
	ResetCredits *ResetCredits
	LimitReached bool
	FetchedAt    time.Time
	Raw          []byte // raw JSON body, for `status -v`
}

// CreditsUsable reports whether credits can cover a request when the weekly
// window is exhausted.
func (u *Usage) CreditsUsable() bool {
	return u.Credits != nil && (u.Credits.Unlimited || u.Credits.HasCredits)
}

// WeeklyExhausted reports whether the weekly window should block new work: its
// utilization is at/above threshold (0..1, e.g. cfg.WeeklyThreshold) and no
// usable credits remain. Shared by the scheduler's ping skip and the continue
// proxy's recovery gate so "weekly is spent" means the same thing everywhere.
func (u *Usage) WeeklyExhausted(threshold float64) bool {
	if u.CreditsUsable() {
		return false
	}
	return u.Weekly.UsedPercent/100 >= threshold
}
