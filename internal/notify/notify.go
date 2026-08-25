// Package notify sends a Telegram message via the Bot API. It is the sole
// alerting channel for this daemon — used only when a provider's OAuth
// refresh token is definitively rejected, i.e. a human needs to log back in.
// A failed or misconfigured send is silently ignored: a missed alert must
// never break the watch loop.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// telegramBaseURL is a var (not const) so tests can point it at a local
// httptest.Server instead of the real Telegram API.
var telegramBaseURL = "https://api.telegram.org"

const sendTimeout = 10 * time.Second

// Config holds the Telegram bot credentials read from config.toml.
type Config struct {
	BotToken string
	ChatID   string
}

// Enabled reports whether both required fields are set.
func (c Config) Enabled() bool { return c.BotToken != "" && c.ChatID != "" }

// Notify sends title and message (joined by a newline) to the configured
// Telegram chat. It is a no-op when cfg is not Enabled(), and it never
// surfaces an error — failures are dropped on the floor by design.
func Notify(cfg Config, title, message string) {
	if !cfg.Enabled() {
		return
	}
	text := title
	if message != "" {
		text += "\n" + message
	}
	body, err := json.Marshal(map[string]string{
		"chat_id": cfg.ChatID,
		"text":    text,
	})
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()

	url := telegramBaseURL + "/bot" + cfg.BotToken + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}
