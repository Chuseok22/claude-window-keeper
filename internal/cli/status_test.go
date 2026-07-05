package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wavever/CCLimitPing/internal/provider"
	"github.com/wavever/CCLimitPing/internal/usage"
)

func TestRunStatusPrintsProgressBeforeReadUsage(t *testing.T) {
	var out bytes.Buffer
	var progress bytes.Buffer

	p := fakeStatusProvider{
		name:  "codex",
		usage: &usage.Usage{Provider: "codex"},
		onRead: func() {
			if !strings.Contains(progress.String(), "Fetching codex usage...\n") {
				t.Fatalf("progress output before ReadUsage = %q, want fetching message", progress.String())
			}
		},
	}

	if err := runStatus(context.Background(), &out, &progress, enText, []provider.Provider{p}, false, false, "used"); err != nil {
		t.Fatalf("runStatus() error = %v", err)
	}
	if !strings.Contains(out.String(), "codex\n") {
		t.Fatalf("status output = %q, want provider usage", out.String())
	}
}

func TestRunStatusJSON(t *testing.T) {
	var out, progress bytes.Buffer

	resets := time.Now().Add(2 * time.Hour)
	providers := []provider.Provider{
		fakeStatusProvider{
			name: "codex",
			usage: &usage.Usage{
				Provider:  "codex",
				Plan:      "pro",
				FiveHour:  usage.Window{UsedPercent: 42.5, ResetsAt: resets, WindowSeconds: 18000},
				FetchedAt: time.Now(),
			},
		},
		fakeStatusProvider{name: "claude", err: errors.New("boom")},
	}

	err := runStatus(context.Background(), &out, &progress, enText, providers, false, true, "used")
	if err == nil {
		t.Fatalf("runStatus() error = nil, want failure for the erroring provider")
	}
	if progress.Len() != 0 {
		t.Fatalf("progress = %q, want no chatter in JSON mode", progress.String())
	}

	var got []statusJSON
	if jsonErr := json.Unmarshal(out.Bytes(), &got); jsonErr != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", jsonErr, out.String())
	}
	if len(got) != 2 {
		t.Fatalf("entries = %d, want 2", len(got))
	}
	if got[0].Provider != "codex" || got[0].Plan != "pro" {
		t.Fatalf("entry[0] = %+v, want codex/pro", got[0])
	}
	if got[0].FiveHour == nil || got[0].FiveHour.UsedPercent != 42.5 || !got[0].FiveHour.Active {
		t.Fatalf("entry[0].five_hour = %+v, want 42.5%% active", got[0].FiveHour)
	}
	if got[0].FiveHour.RemainingPercent != 57.5 {
		t.Fatalf("entry[0].five_hour.remaining_percent = %v, want 57.5", got[0].FiveHour.RemainingPercent)
	}
	if got[0].FiveHour.RemainingSeconds <= 0 {
		t.Fatalf("entry[0].five_hour.remaining_seconds = %d, want > 0", got[0].FiveHour.RemainingSeconds)
	}
	if got[1].Provider != "claude" || got[1].Error == "" {
		t.Fatalf("entry[1] = %+v, want claude with error", got[1])
	}
}

func TestPrintUsageCanShowRemainingPercent(t *testing.T) {
	var out bytes.Buffer
	u := &usage.Usage{
		Provider: "codex",
		FiveHour: usage.Window{UsedPercent: 1},
		Weekly:   usage.Window{UsedPercent: 15},
	}

	printUsage(&out, u, false, "remaining")

	got := out.String()
	if !strings.Contains(got, "99.0% remaining") || !strings.Contains(got, "85.0% remaining") {
		t.Fatalf("status output = %q, want remaining percentages", got)
	}
}

func TestPrintUsageIncludesResetCredits(t *testing.T) {
	var out bytes.Buffer
	u := &usage.Usage{
		Provider: "codex",
		ResetCredits: &usage.ResetCredits{
			AvailableCount: 1,
			Credits: []usage.ResetCredit{
				{
					Status:    "available",
					GrantedAt: time.Date(2026, 6, 17, 17, 38, 0, 0, time.UTC),
					ExpiresAt: time.Date(2026, 7, 17, 17, 38, 0, 0, time.UTC),
				},
			},
		},
	}

	printUsage(&out, u, false, "used")

	got := out.String()
	if !strings.Contains(got, "reset credits 1 reset available") || !strings.Contains(got, "available") {
		t.Fatalf("status output = %q, want reset credit summary", got)
	}
}

type fakeStatusProvider struct {
	name   string
	usage  *usage.Usage
	err    error
	onRead func()
}

func (f fakeStatusProvider) Name() string {
	return f.name
}

func (f fakeStatusProvider) ReadUsage(context.Context) (*usage.Usage, error) {
	if f.onRead != nil {
		f.onRead()
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.usage, nil
}

func (f fakeStatusProvider) Trigger(context.Context, bool) (*provider.TriggerResult, error) {
	return nil, nil
}
