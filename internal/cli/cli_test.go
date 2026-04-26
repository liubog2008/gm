package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
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
	if !strings.Contains(out, `-o|--output-all`) {
		t.Fatalf("expected shell wrapper to preserve output-all, got %q", out)
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
