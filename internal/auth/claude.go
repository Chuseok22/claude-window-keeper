// Package auth loads (and, when necessary, refreshes) the OAuth credentials
// that Claude Code and Codex already store on disk. We reuse the official
// tools' credentials rather than managing our own login.
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// claudeOAuthClientID is Claude Code's public OAuth client id, used for the
// refresh-token grant. Refresh rotates the token in the same store the official
// CLI reads, keeping both in sync. If Anthropic changes this, update here.
const claudeOAuthClientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"

const claudeTokenEndpoint = "https://console.anthropic.com/v1/oauth/token"

// authHTTPClient performs the OAuth refresh requests; swapped in tests (the
// same seam the provider package uses for its usage client).
var authHTTPClient = http.DefaultClient

// ClaudeAuth provides a current Claude access token, reloading from
// ~/.claude/.credentials.json and refreshing via the refresh token when needed.
type ClaudeAuth struct {
	mu      sync.Mutex
	access  string
	refresh string
	account string         // unused placeholder threaded through for write-back
	wrapper map[string]any // full "claudeAiOauth" object, preserved on write-back
}

// NewClaudeAuth returns an empty holder; the token is loaded lazily.
func NewClaudeAuth() *ClaudeAuth { return &ClaudeAuth{} }

// Token returns a cached access token, loading from the store on first use.
func (a *ClaudeAuth) Token(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.access != "" {
		return a.access, nil
	}
	if err := a.loadLocked(); err != nil {
		return "", err
	}
	return a.access, nil
}

// Reload forces a re-read from the store (e.g. the official CLI may have
// refreshed the token since we last read it) and returns the fresh token.
func (a *ClaudeAuth) Reload(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.loadLocked(); err != nil {
		return "", err
	}
	return a.access, nil
}

// Refresh exchanges the refresh token for a new access token and writes the
// rotated credentials back to the store so the official CLI stays in sync.
func (a *ClaudeAuth) Refresh(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.refresh == "" {
		if err := a.loadLocked(); err != nil {
			return "", err
		}
	}
	if a.refresh == "" {
		return "", fmt.Errorf("claude: no refresh token available")
	}

	body, _ := json.Marshal(map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": a.refresh,
		"client_id":     claudeOAuthClientID,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, claudeTokenEndpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := authHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("claude token refresh: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("claude token refresh: HTTP %d", resp.StatusCode)
	}
	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", err
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("claude token refresh: empty access_token")
	}
	a.access = tok.AccessToken
	if tok.RefreshToken != "" {
		a.refresh = tok.RefreshToken
	}
	a.persistLocked(tok.ExpiresIn)
	return a.access, nil
}

func (a *ClaudeAuth) loadLocked() error {
	raw, account, err := readClaudeBlob()
	if err != nil {
		return err
	}
	a.account = account
	var outer map[string]any
	if err := json.Unmarshal(raw, &outer); err != nil {
		return fmt.Errorf("claude credentials: invalid JSON: %w", err)
	}
	// Credentials are stored either as {"claudeAiOauth": {...}} or flat.
	wrapper := outer
	if inner, ok := outer["claudeAiOauth"].(map[string]any); ok {
		wrapper = inner
	}
	a.wrapper = wrapper
	a.access, _ = wrapper["accessToken"].(string)
	a.refresh, _ = wrapper["refreshToken"].(string)
	if a.access == "" {
		return fmt.Errorf("claude credentials: no accessToken found")
	}
	return nil
}

// persistLocked writes the updated tokens back to the same store, preserving
// other fields. Best-effort: a failure here doesn't break the running session,
// it just means we may have to refresh again next time.
func (a *ClaudeAuth) persistLocked(expiresIn int64) {
	if a.wrapper == nil {
		a.wrapper = map[string]any{}
	}
	a.wrapper["accessToken"] = a.access
	a.wrapper["refreshToken"] = a.refresh
	if expiresIn > 0 {
		a.wrapper["expiresAt"] = time.Now().Add(time.Duration(expiresIn) * time.Second).UnixMilli()
	}
	out := map[string]any{"claudeAiOauth": a.wrapper}
	blob, err := json.Marshal(out)
	if err != nil {
		return
	}
	_ = writeClaudeBlob(blob, a.account)
}

// readClaudeBlob returns the raw credentials JSON from ~/.claude/.credentials.json.
func readClaudeBlob() (raw []byte, account string, err error) {
	home, herr := os.UserHomeDir()
	if herr != nil {
		return nil, "", herr
	}
	path := filepath.Join(home, ".claude", ".credentials.json")
	b, ferr := os.ReadFile(path)
	if ferr != nil {
		return nil, "", fmt.Errorf("claude credentials not found at %s: %w", path, ferr)
	}
	return b, "", nil
}

func writeClaudeBlob(blob []byte, _ string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".claude", ".credentials.json")
	return os.WriteFile(path, blob, 0o600)
}
