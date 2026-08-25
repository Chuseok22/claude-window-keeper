package auth

import "errors"

// ErrRefreshRejected means the OAuth server explicitly rejected the refresh
// token (HTTP 400/401 — the token is expired or revoked, not a transient
// failure). Wrap it with fmt.Errorf("...: %w", ErrRefreshRejected) so callers
// can distinguish "log in again" from "try again later".
var ErrRefreshRejected = errors.New("refresh token rejected")
