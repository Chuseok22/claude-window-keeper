// Package notify sends a Discord message via an incoming webhook. It is the
// sole alerting channel for this daemon — used only when a provider's OAuth
// refresh token is definitively rejected, i.e. a human needs to log back in.
// Notify never retries and never blocks the watch loop: on failure it
// returns an error for the caller to log or otherwise surface, but the
// caller is expected to keep looping regardless.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const sendTimeout = 10 * time.Second

// maxErrorBodySnippet caps how much of a failed webhook response body gets
// folded into the returned error, so a misbehaving endpoint can't blow up a
// log line.
const maxErrorBodySnippet = 512

// Config holds the Discord incoming-webhook URL read from the environment.
type Config struct {
	WebhookURL string
}

// Enabled reports whether a webhook URL is configured.
func (c Config) Enabled() bool { return c.WebhookURL != "" }

// Notify posts title and message (joined by a newline) to the configured
// Discord webhook as the message's content. It is a no-op returning nil
// when cfg is not Enabled(). On any failure — a request that never reaches
// Discord, or a non-2xx response — it returns a descriptive error instead
// of swallowing it; the caller decides how to log or otherwise surface
// that error. Notify itself never retries.
func Notify(cfg Config, title, message string) error {
	if !cfg.Enabled() {
		return nil
	}
	text := title
	if message != "" {
		text += "\n" + message
	}
	body, err := json.Marshal(map[string]string{"content": text})
	if err != nil {
		return fmt.Errorf("discord notify: encode payload: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("discord notify: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("discord notify: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySnippet))
		return fmt.Errorf("discord notify: unexpected status %d: %s", resp.StatusCode, snippet)
	}
	return nil
}
