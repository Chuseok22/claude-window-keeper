package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Chuseok22/claude-window-keeper/internal/provider"
	"github.com/Chuseok22/claude-window-keeper/internal/usage"
)

func TestRunStatusPrintsProgressBeforeReadUsage(t *testing.T) {
	var out bytes.Buffer
	var progress bytes.Buffer

	p := fakeStatusProvider{
		name:  "codex",
		usage: &usage.Usage{Provider: "codex"},
		onRead: func() {
			if !strings.Contains(progress.String(), "codex 사용량 조회 중...\n") {
				t.Fatalf("progress output before ReadUsage = %q, want fetching message", progress.String())
			}
		},
	}

	if err := runStatus(context.Background(), &out, &progress, koreanText, []provider.Provider{p}, false, false, "used"); err != nil {
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

	err := runStatus(context.Background(), &out, &progress, koreanText, providers, false, true, "used")
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
	if got[0].Weekly != nil {
		t.Fatalf("entry[0].weekly = %+v, want omitted for a window the provider does not enforce", got[0].Weekly)
	}
	if got[1].Provider != "claude" || got[1].Error == "" {
		t.Fatalf("entry[1] = %+v, want claude with error", got[1])
	}
}

func TestRunStatusLocalizesClaudeSubscriptionAccessError(t *testing.T) {
	var out bytes.Buffer
	p := fakeStatusProvider{name: "claude", err: &provider.ClaudeSubscriptionAccessError{}}

	err := runStatus(context.Background(), &out, io.Discard, koreanText, []provider.Provider{p}, false, false, "used")
	if err == nil {
		t.Fatal("runStatus() error = nil, want provider failure")
	}
	got := out.String()
	if !strings.Contains(got, "Claude 구독 접근이 불가능합니다") ||
		!strings.Contains(got, "만료/갱신 실패") ||
		!strings.Contains(got, "Anthropic API Key") {
		t.Fatalf("localized status output = %q", got)
	}
}

func TestRunStatusJSONPreservesClaudeSubscriptionError(t *testing.T) {
	var out bytes.Buffer
	p := fakeStatusProvider{name: "claude", err: &provider.ClaudeSubscriptionAccessError{}}

	err := runStatus(context.Background(), &out, io.Discard, koreanText, []provider.Provider{p}, false, true, "used")
	if err == nil {
		t.Fatal("runStatus() error = nil, want provider failure")
	}
	var got []statusJSON
	if jsonErr := json.Unmarshal(out.Bytes(), &got); jsonErr != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", jsonErr, out.String())
	}
	want := (&provider.ClaudeSubscriptionAccessError{}).Error()
	if len(got) != 1 || got[0].Provider != "claude" || got[0].Error != want {
		t.Fatalf("JSON status = %+v, want claude error %q", got, want)
	}
}

func TestPrintUsageCanShowRemainingPercent(t *testing.T) {
	var out bytes.Buffer
	u := &usage.Usage{
		Provider: "codex",
		FiveHour: usage.Window{UsedPercent: 1},
		Weekly:   usage.Window{UsedPercent: 15},
	}

	printUsage(&out, koreanText, u, false, "remaining")

	got := out.String()
	if !strings.Contains(got, "99.0% 잔여") || !strings.Contains(got, "85.0% 잔여") {
		t.Fatalf("status output = %q, want remaining percentages", got)
	}
}

func TestPrintUsageMarksMissingWindowNotEnforced(t *testing.T) {
	var out bytes.Buffer
	u := &usage.Usage{
		Provider: "codex",
		// No 5h window: Codex weekly-only regime since 2026-07-12.
		Weekly: usage.Window{
			UsedPercent:   24,
			ResetsAt:      time.Now().Add(24 * time.Hour),
			WindowSeconds: 604800,
		},
	}

	printUsage(&out, koreanText, u, false, "used")

	got := out.String()
	if !strings.Contains(got, "5h     현재 적용되지 않음") {
		t.Fatalf("status output = %q, want missing 5h window marked as not enforced", got)
	}
	if !strings.Contains(got, "24.0% 사용") {
		t.Fatalf("status output = %q, want weekly usage rendered", got)
	}
}

func TestPrintUsageRendersKorean(t *testing.T) {
	var out bytes.Buffer
	u := &usage.Usage{
		Provider: "codex",
		Plan:     "plus",
		// Weekly-only regime: the 5h window is not enforced.
		Weekly: usage.Window{
			UsedPercent:   27,
			ResetsAt:      time.Now().Add(24 * time.Hour),
			WindowSeconds: 604800,
		},
		ResetCredits: &usage.ResetCredits{
			AvailableCount: 1,
			Credits: []usage.ResetCredit{
				{
					Status:    "available",
					GrantedAt: time.Now().Add(-24 * time.Hour),
					ExpiresAt: time.Now().Add(29*24*time.Hour + 2*time.Hour),
				},
			},
		},
	}

	printUsage(&out, koreanText, u, false, "used")

	got := out.String()
	for _, want := range []string{
		"5h     현재 적용되지 않음",
		"주     [",
		"27.0% 사용",
		"후 리셋 (",
		"리셋 크레딧 1개 사용 가능",
		"사용 가능, 발급일",
		"만료일",
		"(남은 기간 29d",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("ko status output = %q, want it to contain %q", got, want)
		}
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
					GrantedAt: time.Now().Add(-24 * time.Hour),
					ExpiresAt: time.Now().Add(29*24*time.Hour + 2*time.Hour),
				},
			},
		},
	}

	printUsage(&out, koreanText, u, false, "used")

	got := out.String()
	if !strings.Contains(got, "리셋 크레딧 1개 사용 가능") || !strings.Contains(got, "사용 가능") {
		t.Fatalf("status output = %q, want reset credit summary", got)
	}
	if !strings.Contains(got, "(남은 기간 29d") {
		t.Fatalf("status output = %q, want remaining lifetime on the expires part", got)
	}
}

func TestResetCreditLineOmitsRemainingWhenRedeemedOrExpired(t *testing.T) {
	redeemed := usage.ResetCredit{
		Status:     "redeemed",
		ExpiresAt:  time.Now().Add(10 * 24 * time.Hour),
		RedeemedAt: time.Now().Add(-time.Hour),
	}
	if line := resetCreditLine(koreanText, redeemed); strings.Contains(line, "(남은 기간 ") {
		t.Fatalf("redeemed credit line = %q, want no remaining lifetime", line)
	}
	expired := usage.ResetCredit{
		Status:    "expired",
		ExpiresAt: time.Now().Add(-time.Hour),
	}
	if line := resetCreditLine(koreanText, expired); strings.Contains(line, "(남은 기간 ") {
		t.Fatalf("expired credit line = %q, want no remaining lifetime", line)
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

func TestFmtZoneRendersOffsetNotAbbreviation(t *testing.T) {
	cases := []struct {
		name string
		zone *time.Location
		want string
	}{
		{"whole hours east", time.FixedZone("CST", 8*3600), "UTC+8"},
		{"whole hours west", time.FixedZone("EST", -5*3600), "UTC-5"},
		{"half-hour offset", time.FixedZone("IST", 5*3600+1800), "UTC+5:30"},
		{"utc", time.UTC, "UTC"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := fmtZone(time.Date(2026, 7, 31, 12, 0, 0, 0, tc.zone)); got != tc.want {
				t.Fatalf("fmtZone() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFmtWindowAndResetCreditCarryTheZone(t *testing.T) {
	zone := fmtZone(time.Now())
	w := usage.Window{UsedPercent: 45, ResetsAt: time.Now().Add(3 * time.Hour), WindowSeconds: 18000}
	if got := fmtWindow(koreanText, w, "used"); !strings.Contains(got, zone) {
		t.Fatalf("window line = %q, want the zone %q", got, zone)
	}
	credit := usage.ResetCredit{Status: "available", ExpiresAt: time.Now().Add(10 * 24 * time.Hour)}
	if got := resetCreditLine(koreanText, credit); !strings.Contains(got, zone) {
		t.Fatalf("credit line = %q, want the zone %q on the expiry", got, zone)
	}
}
