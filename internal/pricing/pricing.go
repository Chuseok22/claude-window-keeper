// Package pricing computes a USD cost for token usage using the LiteLLM pricing
// dataset — the same approach CodexBar/ccusage use. Providers like Codex (on a
// ChatGPT subscription) don't return a USD cost, so we derive the equivalent
// API-rate cost from the per-model rates.
package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Chuseok22/claude-window-keeper/internal/config"
)

// litellmURL is the canonical pricing dataset ccusage/CodexBar pull from.
const litellmURL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"

const cacheTTL = 24 * time.Hour

// Price holds per-token rates (USD).
type Price struct {
	InputPerToken      float64
	CachedReadPerToken float64
	OutputPerToken     float64
}

// Cost returns the USD cost for a request. inputTotal includes the cached
// portion; output already includes reasoning tokens (LiteLLM bills reasoning as
// output, so callers must not add them again).
func (p Price) Cost(inputTotal, cached, output int) float64 {
	nonCached := inputTotal - cached
	if nonCached < 0 {
		nonCached = 0
	}
	cacheRate := p.CachedReadPerToken
	if cacheRate == 0 {
		cacheRate = p.InputPerToken // fall back to input rate when unset
	}
	return float64(nonCached)*p.InputPerToken +
		float64(cached)*cacheRate +
		float64(output)*p.OutputPerToken
}

type entry struct {
	InputCostPerToken       float64 `json:"input_cost_per_token"`
	OutputCostPerToken      float64 `json:"output_cost_per_token"`
	CacheReadInputTokenCost float64 `json:"cache_read_input_token_cost"`
}

// Fetcher loads and caches the LiteLLM dataset.
type Fetcher struct {
	mu     sync.Mutex
	raw    map[string]json.RawMessage
	loaded bool
}

var (
	defOnce sync.Once
	def     *Fetcher
)

// Default returns a process-wide shared fetcher.
func Default() *Fetcher {
	defOnce.Do(func() { def = &Fetcher{} })
	return def
}

// Lookup resolves a model name (with Codex alias / date-suffix fallbacks) to its
// price, loading the dataset on first use.
func (f *Fetcher) Lookup(ctx context.Context, model string) (Price, bool) {
	if model == "" {
		return Price{}, false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.loadLocked(ctx); err != nil {
		return Price{}, false
	}
	for _, key := range candidates(model) {
		rm, ok := f.raw[key]
		if !ok {
			continue
		}
		var e entry
		if json.Unmarshal(rm, &e) == nil && e.InputCostPerToken > 0 {
			return Price{
				InputPerToken:      e.InputCostPerToken,
				CachedReadPerToken: e.CacheReadInputTokenCost,
				OutputPerToken:     e.OutputCostPerToken,
			}, true
		}
	}
	return Price{}, false
}

func (f *Fetcher) loadLocked(ctx context.Context) error {
	if f.loaded {
		return nil
	}
	data, err := readData(ctx)
	if err != nil {
		return err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	f.raw = m
	f.loaded = true
	return nil
}

// readData returns the dataset from the on-disk cache when fresh, otherwise
// fetches it and refreshes the cache. A stale cache is used if the fetch fails.
func readData(ctx context.Context) ([]byte, error) {
	path := cachePath()
	if path != "" {
		if fi, err := os.Stat(path); err == nil && time.Since(fi.ModTime()) < cacheTTL {
			if b, err := os.ReadFile(path); err == nil {
				return b, nil
			}
		}
	}
	b, err := fetch(ctx)
	if err != nil {
		if path != "" {
			if sb, rerr := os.ReadFile(path); rerr == nil {
				return sb, nil // stale fallback
			}
		}
		return nil, err
	}
	if path != "" {
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		_ = os.WriteFile(path, b, 0o644)
	}
	return b, nil
}

func fetch(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, litellmURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("litellm pricing: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func cachePath() string {
	dir, err := config.Dir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "litellm_prices.json")
}

var dateSuffix = regexp.MustCompile(`-\d{4}-\d{2}-\d{2}$`)

// candidates returns model keys to try, in order: exact, date-suffix-stripped,
// and the Codex "-codex" alias removed (e.g. gpt-5-codex -> gpt-5).
func candidates(model string) []string {
	out := []string{model}
	if s := dateSuffix.FindString(model); s != "" {
		out = append(out, strings.TrimSuffix(model, s))
	}
	if strings.Contains(model, "-codex") {
		out = append(out, strings.Replace(model, "-codex", "", 1))
	}
	return out
}
