package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestLocalizedTextUsesChineseForChineseLocale(t *testing.T) {
	setLocale(t, "zh_CN.UTF-8")

	text := localizedText()
	if !strings.Contains(text.pingLong, "参数") {
		t.Fatalf("pingLong = %q, want Chinese help text", text.pingLong)
	}
	if !strings.Contains(text.statusShort, "用量") {
		t.Fatalf("statusShort = %q, want Chinese help text", text.statusShort)
	}
}

func TestLocalizedTextFallsBackToEnglish(t *testing.T) {
	setLocale(t, "C")

	text := localizedText()
	if !strings.Contains(text.pingLong, "Arguments") {
		t.Fatalf("pingLong = %q, want English help text", text.pingLong)
	}
	if !strings.Contains(text.statusShort, "usage") {
		t.Fatalf("statusShort = %q, want English help text", text.statusShort)
	}
}

func TestRootCommandAliases(t *testing.T) {
	setLocale(t, "C")

	root := newRootCmd()
	cases := map[string]string{
		"p":      "ping",
		"sched":  "schedule",
		"s":      "status",
		"w":      "watch",
		"c":      "config",
		"cfg":    "config",
		"v":      "version",
		"ver":    "version",
		"up":     "upgrade",
		"update": "upgrade",
		"rm":     "uninstall",
		"remove": "uninstall",
	}

	for alias, want := range cases {
		cmd, _, err := root.Find([]string{alias})
		if err != nil {
			t.Fatalf("Find(%q) error = %v", alias, err)
		}
		if got := cmd.Name(); got != want {
			t.Fatalf("Find(%q) = %q, want %q", alias, got, want)
		}
	}

	nested := map[string]string{
		"i": "init",
		"p": "path",
	}
	for alias, want := range nested {
		cmd, _, err := root.Find([]string{"c", alias})
		if err != nil {
			t.Fatalf("Find(config %q) error = %v", alias, err)
		}
		if got := cmd.Name(); got != want {
			t.Fatalf("Find(config %q) = %q, want %q", alias, got, want)
		}
	}
}

func TestHelpFlagDescriptionIsLocalized(t *testing.T) {
	setLocale(t, "zh_CN.UTF-8")

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"ping", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "显示此命令的帮助") {
		t.Fatalf("help output = %q, want localized help flag", got)
	}
}

func TestRootHelpLocalizesDefaultCompletionCommand(t *testing.T) {
	setLocale(t, "zh_CN.UTF-8")

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "生成 shell 补全脚本") {
		t.Fatalf("help output = %q, want localized completion command", got)
	}
	if strings.Contains(got, "Generate the autocompletion script") {
		t.Fatalf("help output = %q, still contains default English completion text", got)
	}
}

func TestRootHelpPrintsCommandAliases(t *testing.T) {
	setLocale(t, "C")

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"ping, p",
		"status, s, stat",
		"version, v, ver",
		"upgrade, up, update",
		"uninstall, rm, remove",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("help output = %q, want command alias %q", got, want)
		}
	}
}

func TestConfigHelpPrintsSubcommandAliases(t *testing.T) {
	setLocale(t, "zh_CN.UTF-8")

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"config", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"init, i", "path, p"} {
		if !strings.Contains(got, want) {
			t.Fatalf("help output = %q, want subcommand alias %q", got, want)
		}
	}
}

func setLocale(t *testing.T, locale string) {
	t.Helper()
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANGUAGE", "LANG"} {
		t.Setenv(key, "")
	}
	t.Setenv("LANG", locale)
}
