package gitx

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type Runner interface {
	Run(ctx context.Context, dir string, args ...string) ([]byte, error)
}

type CommandRunner struct {
	Stdout          io.Writer
	Stderr          io.Writer
	StreamGitOutput bool
}

func (r CommandRunner) Run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	if r.StreamGitOutput && shouldStreamOutput(args) {
		cmd.Args = append([]string{"git"}, withProgress(args)...)
		if r.Stdout != nil {
			cmd.Stdout = r.Stdout
		}
		if r.Stderr != nil {
			cmd.Stderr = r.Stderr
		}
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		return nil, nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() == 0 {
			return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func shouldStreamOutput(args []string) bool {
	if len(args) == 0 {
		return false
	}
	if args[0] == "clone" || args[0] == "fetch" {
		return true
	}
	return len(args) >= 4 && args[0] == "--git-dir" && args[2] == "worktree" && args[3] == "add"
}

func withProgress(args []string) []string {
	if len(args) == 0 {
		return args
	}
	switch args[0] {
	case "clone", "fetch":
		if hasArg(args, "--progress") {
			return args
		}
		out := make([]string, 0, len(args)+1)
		out = append(out, args[0], "--progress")
		out = append(out, args[1:]...)
		return out
	default:
		return args
	}
}

func hasArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
