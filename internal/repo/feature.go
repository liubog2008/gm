package repo

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	featureStatusLocal  = "local"
	featureStatusSynced = "synced"
)

type FeatureState struct {
	Status  string   `yaml:"status"`
	Remotes []string `yaml:"remotes"`
}

type FeatureAddOptions struct {
	Name     string
	Base     string
	StartDir string
}

type FeatureSyncOptions struct {
	StartDir string
	Remote   string
}

type FeaturePruneOptions struct {
	StartDir string
	Stderr   io.Writer
}

type FeaturePruneResult struct {
	RemovedPaths []string
	FinalDir     string
}

type managedContext struct {
	repoRoot        string
	bareDir         string
	currentWorktree string
}

func (m *Manager) AddFeatureWorktree(ctx context.Context, opts FeatureAddOptions) (string, error) {
	name := strings.TrimSpace(opts.Name)
	if err := validateWorktreeName(name); err != nil {
		return "", err
	}
	base := strings.TrimSpace(opts.Base)
	if base == "" {
		base = DefaultWorktreeName
	}
	if base != DefaultWorktreeName && base != "upstream/"+DefaultWorktreeName {
		return "", fmt.Errorf("unsupported base %q; expected %q or %q", base, DefaultWorktreeName, "upstream/"+DefaultWorktreeName)
	}

	mctx, err := m.resolveManagedContext(ctx, opts.StartDir)
	if err != nil {
		return "", err
	}

	targetPath := filepath.Join(mctx.repoRoot, name)
	if _, err := os.Stat(targetPath); err == nil {
		ok, err := m.isManagedWorktree(ctx, mctx.bareDir, targetPath)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", fmt.Errorf("%s exists but is not a gm worktree", targetPath)
		}
		branch, err := m.currentBranch(ctx, targetPath)
		if err != nil {
			return "", err
		}
		if branch != name {
			return "", fmt.Errorf("%s exists but is checked out on branch %q", targetPath, branch)
		}
		if err := ensureFeatureState(targetPath); err != nil {
			return "", err
		}
		return targetPath, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	localRef := "refs/heads/" + name
	if m.refExists(ctx, mctx.bareDir, localRef) {
		return "", fmt.Errorf("branch %q already exists without matching worktree", name)
	}

	baseRef := featureBaseRef(base)
	if !m.refExists(ctx, mctx.bareDir, baseRef) {
		return "", fmt.Errorf("base %q does not exist", base)
	}

	if _, err := m.git.Run(ctx, mctx.repoRoot, "--git-dir", mctx.bareDir, "worktree", "add", "-b", name, targetPath, base); err != nil {
		return "", err
	}
	if err := writeFeatureState(targetPath, FeatureState{Status: featureStatusLocal, Remotes: []string{}}); err != nil {
		return "", err
	}
	return targetPath, nil
}

func (m *Manager) SyncFeatureWorktree(ctx context.Context, opts FeatureSyncOptions) (string, error) {
	remote := strings.TrimSpace(opts.Remote)
	if remote == "" {
		remote = "origin"
	}
	if strings.Contains(remote, "/") {
		return "", fmt.Errorf("invalid remote %q", remote)
	}

	mctx, err := m.resolveManagedContext(ctx, opts.StartDir)
	if err != nil {
		return "", err
	}
	if mctx.currentWorktree == "" {
		return "", fmt.Errorf("not inside a gm feature worktree")
	}

	state, err := readFeatureState(mctx.currentWorktree)
	if err != nil {
		return "", err
	}

	branch := filepath.Base(mctx.currentWorktree)
	currentBranch, err := m.currentBranch(ctx, mctx.currentWorktree)
	if err != nil {
		return "", err
	}
	if currentBranch != branch {
		return "", fmt.Errorf("current branch %q does not match worktree name %q", currentBranch, branch)
	}
	if _, err := m.git.Run(ctx, mctx.currentWorktree, "remote", "get-url", remote); err != nil {
		return "", fmt.Errorf("remote %q does not exist", remote)
	}
	if dirty, err := m.worktreeDirty(ctx, mctx.currentWorktree); err != nil {
		return "", err
	} else if dirty {
		return "", fmt.Errorf("current worktree is dirty; commit or stash changes before syncing")
	}

	if _, err := m.git.Run(ctx, mctx.currentWorktree, "push", "-u", remote, "HEAD:"+branch); err != nil {
		return "", err
	}

	state.Status = featureStatusSynced
	state.Remotes = appendUnique(state.Remotes, remote)
	if err := writeFeatureState(mctx.currentWorktree, state); err != nil {
		return "", err
	}
	return remote + "/" + branch, nil
}

func (m *Manager) PruneFeatureWorktrees(ctx context.Context, opts FeaturePruneOptions) (FeaturePruneResult, error) {
	mctx, err := m.resolveManagedContext(ctx, opts.StartDir)
	if err != nil {
		return FeaturePruneResult{}, err
	}
	if _, err := m.git.Run(ctx, mctx.repoRoot, "--git-dir", mctx.bareDir, "fetch", "--all", "--prune"); err != nil {
		return FeaturePruneResult{}, err
	}

	worktrees, err := m.managedWorktrees(ctx, mctx.repoRoot, mctx.bareDir)
	if err != nil {
		return FeaturePruneResult{}, err
	}

	current := filepath.Clean(mctx.currentWorktree)
	candidates := make([]Worktree, 0)
	removeCurrent := false
	for _, wt := range worktrees {
		if filepath.Clean(wt.Path) == filepath.Clean(mctx.bareDir) {
			continue
		}
		state, err := readFeatureState(wt.Path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			warn(opts.Stderr, "skip %s: %v", wt.Path, err)
			continue
		}
		if state.Status != featureStatusSynced || len(state.Remotes) == 0 {
			continue
		}
		branch := filepath.Base(wt.Path)
		if m.anyRemoteBranchExists(ctx, mctx.bareDir, state.Remotes, branch) {
			warn(opts.Stderr, "skip %s: remote branch still exists", wt.Path)
			continue
		}
		if dirty, err := m.worktreeDirty(ctx, wt.Path); err != nil {
			warn(opts.Stderr, "skip %s: %v", wt.Path, err)
			continue
		} else if dirty {
			warn(opts.Stderr, "skip %s: worktree is dirty", wt.Path)
			continue
		}
		candidates = append(candidates, wt)
		if current != "" && filepath.Clean(wt.Path) == current {
			removeCurrent = true
		}
	}

	if removeCurrent {
		if err := os.Chdir(mctx.repoRoot); err != nil {
			return FeaturePruneResult{}, err
		}
	}

	result := FeaturePruneResult{RemovedPaths: make([]string, 0, len(candidates))}
	for _, wt := range candidates {
		if _, err := m.git.Run(ctx, mctx.repoRoot, "--git-dir", mctx.bareDir, "worktree", "remove", "--force", wt.Path); err != nil {
			return FeaturePruneResult{}, err
		}
		result.RemovedPaths = append(result.RemovedPaths, wt.Path)
	}
	if removeCurrent {
		result.FinalDir = mctx.repoRoot
	}
	return result, nil
}

func (m *Manager) resolveManagedContext(ctx context.Context, startDir string) (managedContext, error) {
	if startDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return managedContext{}, err
		}
		startDir = wd
	}
	startDir, err := filepath.Abs(startDir)
	if err != nil {
		return managedContext{}, err
	}

	worktreeRoot, worktreeErr := m.worktreeRoot(ctx, startDir)
	if worktreeErr == nil {
		root, bareDir, err := findManagedRepoRoot(worktreeRoot)
		if err == nil {
			return managedContext{repoRoot: root, bareDir: bareDir, currentWorktree: worktreeRoot}, nil
		}
	}

	root, bareDir, err := findManagedRepoRoot(startDir)
	if err != nil {
		return managedContext{}, fmt.Errorf("not inside a gm managed repo")
	}
	return managedContext{repoRoot: root, bareDir: bareDir}, nil
}

func findManagedRepoRoot(start string) (string, string, error) {
	dir := filepath.Clean(start)
	for {
		bareDir := filepath.Join(dir, ".bare")
		if isManagedRepoRoot(dir, bareDir) {
			return dir, bareDir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", fmt.Errorf("not inside a gm managed repo")
		}
		dir = parent
	}
}

func featureBaseRef(base string) string {
	if base == DefaultWorktreeName {
		return "refs/heads/" + DefaultWorktreeName
	}
	return "refs/remotes/" + base
}

func (m *Manager) managedWorktrees(ctx context.Context, root, bareDir string) ([]Worktree, error) {
	out, err := m.git.Run(ctx, root, "--git-dir", bareDir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parseWorktreeList(out), nil
}

func (m *Manager) worktreeDirty(ctx context.Context, path string) (bool, error) {
	out, err := m.git.Run(ctx, path, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if statusPath(line) == ".gm" {
			continue
		}
		return true, nil
	}
	return false, nil
}

func (m *Manager) anyRemoteBranchExists(ctx context.Context, bareDir string, remotes []string, branch string) bool {
	for _, remote := range remotes {
		remote = strings.TrimSpace(remote)
		if remote == "" {
			continue
		}
		if m.refExists(ctx, bareDir, "refs/remotes/"+remote+"/"+branch) {
			return true
		}
	}
	return false
}

func ensureFeatureState(worktreePath string) error {
	if _, err := readFeatureState(worktreePath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return writeFeatureState(worktreePath, FeatureState{Status: featureStatusLocal, Remotes: []string{}})
}

func readFeatureState(worktreePath string) (FeatureState, error) {
	path := filepath.Join(worktreePath, ".gm")
	info, err := os.Stat(path)
	if err != nil {
		return FeatureState{}, err
	}
	if !info.Mode().IsRegular() {
		return FeatureState{}, fmt.Errorf("%s is not a regular file", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return FeatureState{}, err
	}
	var state FeatureState
	if err := yaml.Unmarshal(data, &state); err != nil {
		return FeatureState{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return state, nil
}

func writeFeatureState(worktreePath string, state FeatureState) error {
	if state.Status == "" {
		state.Status = featureStatusLocal
	}
	if state.Remotes == nil {
		state.Remotes = []string{}
	}
	data, err := yaml.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(worktreePath, ".gm"), data, 0o644)
}

func appendUnique(items []string, item string) []string {
	for _, existing := range items {
		if existing == item {
			return items
		}
	}
	return append(items, item)
}

func warn(w io.Writer, format string, args ...interface{}) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprintf(w, format+"\n", args...)
}

func statusPath(line string) string {
	if len(line) < 4 {
		return strings.TrimSpace(line)
	}
	path := strings.TrimSpace(line[3:])
	if idx := strings.Index(path, " -> "); idx >= 0 {
		path = strings.TrimSpace(path[idx+4:])
	}
	return path
}
