package gitx

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandRunnerStreamsCloneOutput(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	source := filepath.Join(tmp, "source")
	target := filepath.Join(tmp, "target.git")

	mustGit(t, ctx, tmp, "init", source)
	mustGit(t, ctx, source, "config", "user.email", "test@example.com")
	mustGit(t, ctx, source, "config", "user.name", "Test User")
	mustGit(t, ctx, source, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("# demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, ctx, source, "add", "README.md")
	mustGit(t, ctx, source, "commit", "-m", "initial")

	var output bytes.Buffer
	runner := CommandRunner{
		Stdout:          &output,
		Stderr:          &output,
		StreamGitOutput: true,
	}
	if _, err := runner.Run(ctx, tmp, "clone", source, target); err != nil {
		t.Fatal(err)
	}

	got := output.String()
	if !strings.Contains(got, "Cloning into bare repository") && !strings.Contains(got, "Cloning into") {
		t.Fatalf("expected clone output, got %q", got)
	}
}

func mustGit(t *testing.T, ctx context.Context, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
