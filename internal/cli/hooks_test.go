package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func readJSONFile(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("%s is invalid JSON: %v", path, err)
	}
	return m
}

func eventGroups(t *testing.T, root map[string]any, event string) []any {
	t.Helper()
	hooks, _ := root["hooks"].(map[string]any)
	if hooks == nil {
		return nil
	}
	arr, _ := hooks[event].([]any)
	return arr
}

func TestApplyHooksInstallCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude", "settings.json")
	spec := claudeSpec("/usr/local/bin/limitping")

	changed, err := applyHooks(path, spec, true)
	if err != nil {
		t.Fatalf("applyHooks: %v", err)
	}
	if !changed {
		t.Fatal("install reported no change on a fresh file")
	}

	root := readJSONFile(t, path)
	for _, event := range spec.events {
		groups := eventGroups(t, root, event)
		if len(groups) != 1 {
			t.Fatalf("%s has %d groups, want exactly our one", event, len(groups))
		}
		group := groups[0].(map[string]any)
		if group["matcher"] != "*" || !groupContainsOurs(group, spec.isOurs) {
			t.Fatalf("%s group = %v, want our matcher-* group", event, group)
		}
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatal("a .bak was created for a file that did not exist")
	}
}

func TestApplyHooksPreservesForeignSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	existing := `{
		"model": "opus",
		"hooks": {
			"Stop": [
				{"matcher": "*", "hooks": [{"type": "command", "command": "other-tool", "args": ["notify"]}]}
			]
		}
	}`
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := claudeSpec("/usr/local/bin/limitping")

	if _, err := applyHooks(path, spec, true); err != nil {
		t.Fatalf("install: %v", err)
	}

	root := readJSONFile(t, path)
	if root["model"] != "opus" {
		t.Fatalf("unrelated top-level setting lost: %v", root)
	}
	stop := eventGroups(t, root, "Stop")
	if len(stop) != 2 {
		t.Fatalf("Stop has %d groups, want foreign + ours", len(stop))
	}
	// The pre-existing backup must hold the original content.
	bak, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("no .bak written: %v", err)
	}
	if string(bak) != existing {
		t.Fatalf(".bak = %s, want the pre-install content", bak)
	}

	// Uninstall strips only ours, leaving the foreign hook and settings.
	if _, err := applyHooks(path, spec, false); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	root = readJSONFile(t, path)
	if root["model"] != "opus" {
		t.Fatalf("unrelated setting lost on uninstall: %v", root)
	}
	stop = eventGroups(t, root, "Stop")
	if len(stop) != 1 || groupContainsOurs(stop[0].(map[string]any), spec.isOurs) {
		t.Fatalf("Stop after uninstall = %v, want only the foreign hook", stop)
	}
	if groups := eventGroups(t, root, "UserPromptSubmit"); len(groups) != 0 {
		t.Fatalf("UserPromptSubmit not cleaned up: %v", groups)
	}
}

func TestApplyHooksInstallIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	spec := codexSpec("/usr/local/bin/limitping")

	for i := 0; i < 3; i++ {
		if _, err := applyHooks(path, spec, true); err != nil {
			t.Fatalf("install #%d: %v", i+1, err)
		}
	}

	root := readJSONFile(t, path)
	for _, event := range spec.events {
		if groups := eventGroups(t, root, event); len(groups) != 1 {
			t.Fatalf("%s has %d groups after repeated installs, want 1", event, len(groups))
		}
	}
}

func TestApplyHooksUninstallRemovesEmptyHooksKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	spec := codexSpec("/usr/local/bin/limitping")

	if _, err := applyHooks(path, spec, true); err != nil {
		t.Fatal(err)
	}
	if _, err := applyHooks(path, spec, false); err != nil {
		t.Fatal(err)
	}

	root := readJSONFile(t, path)
	if _, ok := root["hooks"]; ok {
		t.Fatalf("empty hooks key left behind: %v", root)
	}
}

func TestApplyHooksUninstallWithoutFileIsNoop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")

	changed, err := applyHooks(path, codexSpec("limitping"), false)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if changed {
		t.Fatal("uninstall reported a change without a file")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("uninstall created the hooks file")
	}
}

func TestApplyHooksRejectsMalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{broken`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := applyHooks(path, claudeSpec("limitping"), true); err == nil {
		t.Fatal("applyHooks overwrote a malformed settings file")
	}
	// The broken file must be untouched, not clobbered.
	data, _ := os.ReadFile(path)
	if string(data) != `{broken` {
		t.Fatalf("malformed file was modified: %s", data)
	}
}

func TestProviderHookSpecsRecognizeTheirOwnHandlers(t *testing.T) {
	claude := claudeSpec("/usr/local/bin/limitping")
	if !claude.isOurs(claude.handler) {
		t.Fatal("claude spec does not recognize its own handler")
	}
	if claude.isOurs(map[string]any{"args": []any{"hook", "codex"}}) {
		t.Fatal("claude spec claims the codex handler")
	}

	codex := codexSpec("/usr/local/bin/limitping")
	if !codex.isOurs(codex.handler) {
		t.Fatal("codex spec does not recognize its own handler")
	}
	if codex.isOurs(map[string]any{"command": "other-tool notify"}) {
		t.Fatal("codex spec claims a foreign handler")
	}
}

func TestQuoteCmd(t *testing.T) {
	if got := quoteCmd("/usr/local/bin/limitping"); got != "/usr/local/bin/limitping" {
		t.Fatalf("quoteCmd = %q, want unquoted", got)
	}
	if got := quoteCmd("/Users/x y/limitping"); got != `"/Users/x y/limitping"` {
		t.Fatalf("quoteCmd = %q, want quoted", got)
	}
}
