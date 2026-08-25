package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestLocalizedTextIsKorean(t *testing.T) {
	text := localizedText()
	if !strings.Contains(text.pingLong, "인자") {
		t.Fatalf("pingLong = %q, want Korean help text", text.pingLong)
	}
	if !strings.Contains(text.statusShort, "사용량") {
		t.Fatalf("statusShort = %q, want Korean help text", text.statusShort)
	}
}

func TestRootCommandAliases(t *testing.T) {
	root := newRootCmd()
	cases := map[string]string{
		"p":   "ping",
		"s":   "status",
		"w":   "watch",
		"c":   "config",
		"cfg": "config",
		"v":   "version",
		"ver": "version",
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
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"ping", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "이 명령어의 도움말 표시") {
		t.Fatalf("help output = %q, want localized help flag", got)
	}
}

func TestRootHelpLocalizesDefaultCompletionCommand(t *testing.T) {
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "셸 자동완성 스크립트 생성") {
		t.Fatalf("help output = %q, want localized completion command", got)
	}
	if strings.Contains(got, "Generate the autocompletion script") {
		t.Fatalf("help output = %q, still contains default English completion text", got)
	}
}

func TestRootHelpPrintsCommandAliases(t *testing.T) {
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
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("help output = %q, want command alias %q", got, want)
		}
	}
}

func TestConfigHelpPrintsSubcommandAliases(t *testing.T) {
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
