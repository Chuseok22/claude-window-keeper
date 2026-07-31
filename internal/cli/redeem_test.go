package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/wavever/CCLimitPing/internal/provider"
	"github.com/wavever/CCLimitPing/internal/usage"
)

func TestNextRedeemableCreditIgnoresTheTimingPolicy(t *testing.T) {
	now := time.Now()
	// Far from expiry, so the automatic policy would decline — an explicit
	// `redeem` still spends it.
	u := &usage.Usage{ResetCredits: &usage.ResetCredits{Credits: []usage.ResetCredit{
		{Status: "available", ExpiresAt: now.Add(20 * 24 * time.Hour)},
		{Status: "redeemed", ExpiresAt: now.Add(time.Hour), RedeemedAt: now},
	}}}
	if _, ok := u.ResetCreditToRedeem(now); ok {
		t.Fatal("precondition: the auto policy should decline this credit")
	}
	got, ok := nextRedeemableCredit(u, now)
	if !ok || !got.ExpiresAt.Equal(now.Add(20*24*time.Hour)) {
		t.Fatalf("nextRedeemableCredit() = %v/%t, want the available credit", got.ExpiresAt, ok)
	}
}

func TestNextRedeemableCreditFallsBackToTheCount(t *testing.T) {
	// The detail endpoint is private; when only the count survives, redeeming
	// must still be possible.
	u := &usage.Usage{ResetCredits: &usage.ResetCredits{AvailableCount: 1}}
	credit, ok := nextRedeemableCredit(u, time.Now())
	if !ok {
		t.Fatal("nextRedeemableCredit() = false, want the count to be trusted")
	}
	if !credit.ExpiresAt.IsZero() {
		t.Fatalf("credit = %+v, want an unknown expiry", credit)
	}

	none := &usage.Usage{ResetCredits: &usage.ResetCredits{}}
	if _, ok := nextRedeemableCredit(none, time.Now()); ok {
		t.Fatal("nextRedeemableCredit() = true with no credits at all")
	}
}

func TestRedeemOutcomeTextCoversEveryBackendOutcome(t *testing.T) {
	for _, outcome := range []string{
		provider.RedeemReset,
		provider.RedeemNothingToReset,
		provider.RedeemNoCredit,
		provider.RedeemAlreadyRedeemed,
	} {
		for _, text := range []cliText{enText, zhText} {
			got := redeemOutcomeText(text, outcome)
			if got == "" || got == outcome {
				t.Fatalf("redeemOutcomeText(%q) = %q, want a translated sentence", outcome, got)
			}
		}
	}
	// An outcome we don't know about must surface as-is rather than read as a
	// successful redemption.
	if got := redeemOutcomeText(enText, "brand_new_code"); !strings.Contains(got, "brand_new_code") {
		t.Fatalf("unknown outcome = %q, want the raw code reported", got)
	}
}
