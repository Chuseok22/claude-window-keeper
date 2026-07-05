package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wavever/CCLimitPing/internal/config"
	"github.com/wavever/CCLimitPing/internal/provider"
	"github.com/wavever/CCLimitPing/internal/usage"
)

func newStatusCmd() *cobra.Command {
	var verbose bool
	var jsonOut bool
	text := localizedText()
	cmd := &cobra.Command{
		Use:     "status",
		Aliases: []string{"s", "stat"},
		Short:   text.statusShort,
		Long:    text.statusLong,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			providers := enabledProviders(cfg)
			if len(providers) == 0 {
				return fmt.Errorf("no providers enabled in config")
			}
			return runStatus(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), text, providers, verbose, jsonOut, cfg.UsageDisplay)
		},
	}
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, text.statusVerboseFlag)
	cmd.Flags().BoolVar(&jsonOut, "json", false, text.statusJSONFlag)
	return cmd
}

func runStatus(ctx context.Context, out, progress io.Writer, text cliText, providers []provider.Provider, verbose, jsonOut bool, display string) error {
	if progress == nil {
		progress = io.Discard
	}
	display = normalizeUsageDisplay(display)
	// In JSON mode keep stdout a single valid document: suppress the
	// "Fetching..." progress chatter that would otherwise interleave.
	if jsonOut {
		progress = io.Discard
	}
	failed := 0
	entries := make([]statusJSON, 0, len(providers))
	for _, p := range providers {
		if text.statusFetchingFmt != "" {
			fmt.Fprintf(progress, text.statusFetchingFmt, p.Name())
		}
		readCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		u, err := p.ReadUsage(readCtx)
		cancel()
		if err != nil {
			failed++
			if jsonOut {
				entries = append(entries, statusJSON{Provider: p.Name(), Error: err.Error()})
				continue
			}
			fmt.Fprintf(out, "%-7s  error: %v\n", p.Name(), err)
			continue
		}
		if jsonOut {
			entries = append(entries, newStatusJSON(u, verbose))
			continue
		}
		printUsage(out, u, verbose, display)
	}
	if jsonOut {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(entries); err != nil {
			return err
		}
	}
	if failed > 0 {
		return fmt.Errorf("status failed for %d provider(s)", failed)
	}
	return nil
}

// statusJSON is the stable, documented shape emitted by `status --json`. It is
// decoupled from usage.Usage so the internal model can evolve without breaking
// scripts that consume this output.
type statusJSON struct {
	Provider     string            `json:"provider"`
	Plan         string            `json:"plan,omitempty"`
	FiveHour     *windowJSON       `json:"five_hour,omitempty"`
	Weekly       *windowJSON       `json:"weekly,omitempty"`
	Credits      *creditsJSON      `json:"credits,omitempty"`
	ResetCredits *resetCreditsJSON `json:"reset_credits,omitempty"`
	LimitReached bool              `json:"limit_reached"`
	FetchedAt    string            `json:"fetched_at,omitempty"`
	Raw          json.RawMessage   `json:"raw,omitempty"`
	Error        string            `json:"error,omitempty"`
}

type windowJSON struct {
	UsedPercent      float64 `json:"used_percent"`
	RemainingPercent float64 `json:"remaining_percent"`
	Active           bool    `json:"active"`
	ResetsAt         string  `json:"resets_at,omitempty"`
	RemainingSeconds int     `json:"remaining_seconds"`
	WindowSeconds    int     `json:"window_seconds,omitempty"`
}

type creditsJSON struct {
	HasCredits bool   `json:"has_credits"`
	Unlimited  bool   `json:"unlimited"`
	Balance    string `json:"balance,omitempty"`
}

type resetCreditsJSON struct {
	AvailableCount int               `json:"available_count"`
	Credits        []resetCreditJSON `json:"credits,omitempty"`
}

type resetCreditJSON struct {
	Status     string `json:"status,omitempty"`
	GrantedAt  string `json:"granted_at,omitempty"`
	ExpiresAt  string `json:"expires_at,omitempty"`
	RedeemedAt string `json:"redeemed_at,omitempty"`
}

func newStatusJSON(u *usage.Usage, verbose bool) statusJSON {
	s := statusJSON{
		Provider:     u.Provider,
		Plan:         u.Plan,
		FiveHour:     newWindowJSON(u.FiveHour),
		Weekly:       newWindowJSON(u.Weekly),
		LimitReached: u.LimitReached,
	}
	if !u.FetchedAt.IsZero() {
		s.FetchedAt = u.FetchedAt.Format(time.RFC3339)
	}
	if u.Credits != nil {
		s.Credits = &creditsJSON{
			HasCredits: u.Credits.HasCredits,
			Unlimited:  u.Credits.Unlimited,
			Balance:    u.Credits.Balance,
		}
	}
	if u.ResetCredits != nil {
		s.ResetCredits = newResetCreditsJSON(u.ResetCredits)
	}
	if verbose && json.Valid(u.Raw) {
		s.Raw = json.RawMessage(u.Raw)
	}
	return s
}

func newWindowJSON(w usage.Window) *windowJSON {
	j := &windowJSON{
		UsedPercent:      w.UsedPercent,
		RemainingPercent: remainingPercent(w.UsedPercent),
		Active:           w.Active(),
		RemainingSeconds: int(w.Remaining().Seconds()),
		WindowSeconds:    w.WindowSeconds,
	}
	if !w.ResetsAt.IsZero() {
		j.ResetsAt = w.ResetsAt.Format(time.RFC3339)
	}
	return j
}

func newResetCreditsJSON(rc *usage.ResetCredits) *resetCreditsJSON {
	out := &resetCreditsJSON{
		AvailableCount: rc.AvailableCount,
		Credits:        make([]resetCreditJSON, 0, len(rc.Credits)),
	}
	for _, c := range rc.Credits {
		out.Credits = append(out.Credits, resetCreditJSON{
			Status:     c.Status,
			GrantedAt:  timeJSON(c.GrantedAt),
			ExpiresAt:  timeJSON(c.ExpiresAt),
			RedeemedAt: timeJSON(c.RedeemedAt),
		})
	}
	return out
}

func timeJSON(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func printUsage(out io.Writer, u *usage.Usage, verbose bool, display string) {
	display = normalizeUsageDisplay(display)
	plan := u.Plan
	if plan != "" {
		plan = " (" + plan + ")"
	}
	fmt.Fprintf(out, "%s%s\n", u.Provider, plan)
	fmt.Fprintf(out, "  5h     %s\n", fmtWindow(u.FiveHour, display))
	fmt.Fprintf(out, "  weekly %s\n", fmtWindow(u.Weekly, display))
	if u.Credits != nil && (u.Credits.HasCredits || u.Credits.Unlimited) {
		if u.Credits.Unlimited {
			fmt.Fprintf(out, "  credits unlimited\n")
		} else {
			fmt.Fprintf(out, "  credits %s\n", u.Credits.Balance)
		}
	}
	printResetCredits(out, u.ResetCredits)
	if verbose {
		fmt.Fprintf(out, "  raw: %s\n", string(u.Raw))
	}
	fmt.Fprintln(out)
}

func printResetCredits(out io.Writer, rc *usage.ResetCredits) {
	if rc == nil || (rc.AvailableCount == 0 && len(rc.Credits) == 0) {
		return
	}
	countWord := "resets"
	if rc.AvailableCount == 1 {
		countWord = "reset"
	}
	fmt.Fprintf(out, "  reset credits %d %s available\n", rc.AvailableCount, countWord)
	for _, c := range rc.Credits {
		line := resetCreditLine(c)
		if line != "" {
			fmt.Fprintf(out, "    - %s\n", line)
		}
	}
}

func resetCreditLine(c usage.ResetCredit) string {
	status := c.Status
	if status == "" {
		switch {
		case !c.RedeemedAt.IsZero():
			status = "redeemed"
		case !c.ExpiresAt.IsZero() && time.Now().After(c.ExpiresAt):
			status = "expired"
		default:
			status = "available"
		}
	}
	parts := []string{status}
	if !c.GrantedAt.IsZero() {
		parts = append(parts, "granted "+c.GrantedAt.Local().Format("Jan 02 15:04"))
	}
	if !c.ExpiresAt.IsZero() {
		parts = append(parts, "expires "+c.ExpiresAt.Local().Format("Jan 02 15:04"))
	}
	if !c.RedeemedAt.IsZero() {
		parts = append(parts, "redeemed "+c.RedeemedAt.Local().Format("Jan 02 15:04"))
	}
	return strings.Join(parts, ", ")
}

func fmtWindow(w usage.Window, display string) string {
	display = normalizeUsageDisplay(display)
	pct := displayedPercent(w, display)
	bar := usageBar(pct)
	if w.ResetsAt.IsZero() {
		return fmt.Sprintf("%s %5.1f%% %-9s (no active window)", bar, pct, display)
	}
	return fmt.Sprintf("%s %5.1f%% %-9s resets in %-8s (%s)",
		bar, pct, display, fmtDur(w.Remaining()), w.ResetsAt.Local().Format("Mon 15:04"))
}

func normalizeUsageDisplay(display string) string {
	if display == "remaining" {
		return "remaining"
	}
	return "used"
}

func displayedPercent(w usage.Window, display string) float64 {
	if normalizeUsageDisplay(display) == "remaining" {
		return remainingPercent(w.UsedPercent)
	}
	return w.UsedPercent
}

func remainingPercent(used float64) float64 {
	remaining := 100 - used
	if remaining < 0 {
		return 0
	}
	if remaining > 100 {
		return 100
	}
	return remaining
}

func usageBar(pct float64) string {
	const width = 10
	filled := int(pct/100*width + 0.5)
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	b := make([]rune, width)
	for i := range b {
		if i < filled {
			b[i] = '█'
		} else {
			b[i] = '░'
		}
	}
	return "[" + string(b) + "]"
}

func fmtDur(d time.Duration) string {
	if d <= 0 {
		return "now"
	}
	d = d.Round(time.Minute)
	h := d / time.Hour
	m := (d % time.Hour) / time.Minute
	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}
