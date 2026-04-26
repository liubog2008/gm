package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunNoArgsRequiresConfigOrBase(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Run(context.Background(), []string{"--config", configPath}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "missing base dir") {
		t.Fatalf("expected missing base dir error, got %v", err)
	}
}

func TestRunUnknownSubcommandReturnsUsage(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Run(context.Background(), []string{"nope"}, &stdout, &stderr)
	if !errors.Is(err, errUsage) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if !strings.Contains(err.Error(), `unknown subcommand "nope"`) {
		t.Fatalf("expected unknown subcommand error, got %v", err)
	}
}

func TestRunFlagParseErrorReturnsUsage(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Run(context.Background(), []string{"--no-such-flag"}, &stdout, &stderr)
	if !errors.Is(err, errUsage) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if !strings.Contains(stderr.String(), "unknown flag") {
		t.Fatalf("expected flag error output, got %q", stderr.String())
	}
}

func TestExitCode(t *testing.T) {
	if got := ExitCode(nil); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
	if got := ExitCode(errUsage); got != 2 {
		t.Fatalf("expected 2, got %d", got)
	}
	if got := ExitCode(errors.New("boom")); got != 1 {
		t.Fatalf("expected 1, got %d", got)
	}
}

func TestRunRemovedListCommandReturnsUsage(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Run(context.Background(), []string{"list"}, &stdout, &stderr)
	if !errors.Is(err, errUsage) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if !strings.Contains(err.Error(), `unknown subcommand "list"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunInitZshPrintsShellIntegration(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Run(context.Background(), []string{"init", "zsh"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "command gm") {
		t.Fatalf("expected shell wrapper to call command gm, got %q", out)
	}
	if !strings.Contains(out, "builtin cd --") {
		t.Fatalf("expected shell wrapper to cd, got %q", out)
	}
	if strings.Contains(out, "_gm_should_cd") {
		t.Fatalf("expected shell wrapper to avoid command-specific cd logic, got %q", out)
	}
	if !strings.Contains(out, shellIntegrationEnv+"=1") {
		t.Fatalf("expected shell wrapper to enable shell integration protocol, got %q", out)
	}
}

func TestShellIntegrationFeatPrunePrintsRemovedPathWithoutCd(t *testing.T) {
	tmp := t.TempDir()
	start := filepath.Join(tmp, "start")
	if err := os.Mkdir(start, 0o755); err != nil {
		t.Fatal(err)
	}
	removed := filepath.Join(tmp, "removed")

	stdout, pwd := runShellIntegration(t, start, removed)
	if stdout != removed+"\n" {
		t.Fatalf("stdout = %q, want %q", stdout, removed+"\n")
	}
	if strings.TrimSpace(pwd) != start {
		t.Fatalf("pwd = %q, want %q", strings.TrimSpace(pwd), start)
	}
}

func TestShellIntegrationFeatPruneCdsToFinalDir(t *testing.T) {
	tmp := t.TempDir()
	start := filepath.Join(tmp, "start")
	finalDir := filepath.Join(tmp, "repo")
	if err := os.Mkdir(start, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(finalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	removed := filepath.Join(tmp, "removed")

	stdout, pwd := runShellIntegration(t, start, removed+"\n"+shellCDPrefix+finalDir)
	if stdout != removed+"\n" {
		t.Fatalf("stdout = %q, want %q", stdout, removed+"\n")
	}
	if strings.TrimSpace(pwd) != finalDir {
		t.Fatalf("pwd = %q, want %q", strings.TrimSpace(pwd), finalDir)
	}
}

func TestRunInitRejectsUnsupportedShell(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Run(context.Background(), []string{"init", "fish"}, &stdout, &stderr)
	if !errors.Is(err, errUsage) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if !strings.Contains(err.Error(), `unsupported shell "fish"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func runShellIntegration(t *testing.T, startDir, fakeOutput string) (string, string) {
	t.Helper()

	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	if err := os.Mkdir(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeGM := filepath.Join(binDir, "gm")
	if err := os.WriteFile(fakeGM, []byte("#!/bin/sh\nprintf '%s\\n' \"$GM_FAKE_OUT\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	stdoutPath := filepath.Join(tmp, "stdout")
	pwdPath := filepath.Join(tmp, "pwd")
	script := shellInitScript("bash") + `
gm feat prune > "$GM_STDOUT"
pwd > "$GM_PWD"
`
	cmd := exec.Command("bash", "-c", script)
	cmd.Dir = startDir
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GM_FAKE_OUT="+fakeOutput,
		"GM_STDOUT="+stdoutPath,
		"GM_PWD="+pwdPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("shell integration failed: %v\n%s", err, output)
	}

	stdout, err := os.ReadFile(stdoutPath)
	if err != nil {
		t.Fatal(err)
	}
	pwd, err := os.ReadFile(pwdPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(stdout), string(pwd)
}

func TestRunFeatAddRejectsUnsupportedBaseAsUsage(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Run(context.Background(), []string{"--base", t.TempDir(), "feat", "add", "feat-login", "origin/main"}, &stdout, &stderr)
	if !errors.Is(err, errUsage) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if !strings.Contains(err.Error(), `unsupported base "origin/main"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunFeatSyncRejectsTooManyArgsAsUsage(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Run(context.Background(), []string{"--base", t.TempDir(), "feat", "sync", "origin", "upstream"}, &stdout, &stderr)
	if !errors.Is(err, errUsage) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if !strings.Contains(err.Error(), "feat sync requires [remote]") {
		t.Fatalf("unexpected error: %v", err)
	}
}
