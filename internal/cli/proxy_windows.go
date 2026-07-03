//go:build windows

package cli

import (
	"context"
	"errors"
	"io"

	"github.com/wavever/CCLimitPing/internal/config"
)

// runContinueProxy is unsupported on Windows: the proxy relies on a Unix PTY and
// raw-terminal handling. The neutral logic in proxy.go still compiles so tests
// run everywhere.
func runContinueProxy(_ context.Context, _ io.Writer, _ string, _ []string, _ config.Config) error {
	return errors.New("limitping continue (interactive proxy) is not supported on Windows yet")
}
