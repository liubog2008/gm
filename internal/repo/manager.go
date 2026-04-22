package repo

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/liubog2008/gm/internal/gitx"
)

const DefaultWorktreeName = "main"

type Worktree struct {
	Name     string
	Path     string
	Branch   string
	Detached bool
	Head     string
}

type ManagedRepo struct {
	Key       string
	Root      string
	Host      string
	RepoPath  string
	RemoteURL string
	Worktrees []Worktree
	Legacy    bool
}

type Manager struct {
	baseDir string
	git     gitx.Runner
}

func NewManager(baseDir string, git gitx.Runner) *Manager {
	return &Manager{baseDir: baseDir, git: git}
}

func (m *Manager) ConvertRepo(ctx context.Context, sourcePath, worktreeName string) (string, error) {
	sourceRoot, err := m.worktreeRoot(ctx, sourcePath)
	if err != nil {
		return "", err
	}

	remoteURL, err := m.remoteURL(ctx, sourceRoot)
	if err != nil {
		return "", err
	}

	loc, err := ParseLocator(m.baseDir, remoteURL)
	if err != nil {
		return "", err
	}

	sourceRoot, err = filepath.Abs(sourceRoot)
	if err != nil {
		return "", err
	}
	if filepath.Clean(sourceRoot) == filepath.Clean(loc.Root) || strings.HasPrefix(filepath.Clean(sourceRoot)+string(os.PathSeparator), filepath.Clean(loc.Root)+string(os.PathSeparator)) {
		return "", fmt.Errorf("%s is already inside managed root %s", sourceRoot, loc.Root)
	}

	currentBranch, err := m.currentBranch(ctx, sourceRoot)
	if err != nil {
		return "", err
	}
	defaultBranch, err := m.sourceDefaultBranch(ctx, sourceRoot)
	if err != nil {
		return "", err
	}
	head, err := m.headCommit(ctx, sourceRoot)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(loc.Root, 0o755); err != nil {
		return "", err
	}

	entries, err := os.ReadDir(loc.Root)
	if err != nil {
		return "", err
	}
	if len(entries) != 0 {
		return "", fmt.Errorf("managed repo root already exists: %s", loc.Root)
	}

	bareDir := filepath.Join(loc.Root, ".bare")
	if _, err := m.git.Run(ctx, m.baseDir, "clone", "--bare", sourceRoot, bareDir); err != nil {
		return "", err
	}
	if _, err := m.git.Run(ctx, loc.Root, "--git-dir", bareDir, "remote", "set-url", "origin", remoteURL); err != nil {
		return "", err
	}
	if err := ensureGitPointer(loc.Root, bareDir); err != nil {
		return "", err
	}
	if worktreeName == "" {
		worktreeName = defaultConvertWorktreeName(currentBranch, defaultBranch)
	}
	if err := validateWorktreeName(worktreeName); err != nil {
		return "", err
	}

	targetPath := filepath.Join(loc.Root, worktreeName)
	if currentBranch == "" {
		_, err = m.git.Run(ctx, loc.Root, "--git-dir", bareDir, "worktree", "add", "--detach", targetPath, head)
	} else {
		_, err = m.git.Run(ctx, loc.Root, "--git-dir", bareDir, "worktree", "add", targetPath, currentBranch)
	}
	if err != nil {
		return "", err
	}

	if err := syncWorktreeSnapshot(sourceRoot, targetPath); err != nil {
		return "", err
	}

	return targetPath, nil
}

func (m *Manager) EnsureRepo(ctx context.Context, rawURL, worktreeName string) (string, error) {
	if err := validateWorktreeName(worktreeName); err != nil {
		return "", err
	}

	loc, err := ParseLocator(m.baseDir, rawURL)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(loc.Root, 0o755); err != nil {
		return "", err
	}

	bareDir := filepath.Join(loc.Root, ".bare")
	if _, err := os.Stat(bareDir); os.IsNotExist(err) {
		if _, err := m.git.Run(ctx, m.baseDir, "clone", "--bare", rawURL, bareDir); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}

	if err := ensureGitPointer(loc.Root, bareDir); err != nil {
		return "", err
	}

	if _, err := m.git.Run(ctx, loc.Root, "--git-dir", bareDir, "fetch", "--all", "--prune"); err != nil {
		return "", err
	}

	defaultBranch, err := m.defaultBranch(ctx, bareDir)
	if err != nil {
		return "", err
	}

	targetPath := filepath.Join(loc.Root, worktreeName)
	if _, err := os.Stat(targetPath); err == nil {
		ok, err := m.isManagedWorktree(ctx, bareDir, targetPath)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", fmt.Errorf("%s exists but is not a worktree managed by %s", targetPath, bareDir)
		}
		return targetPath, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	if worktreeName == DefaultWorktreeName {
		if _, err := m.git.Run(ctx, loc.Root, "--git-dir", bareDir, "worktree", "add", targetPath, defaultBranch); err != nil {
			return "", err
		}
		return targetPath, nil
	}

	if err := m.addNamedWorktree(ctx, bareDir, targetPath, worktreeName, defaultBranch); err != nil {
		return "", err
	}

	return targetPath, nil
}

func (m *Manager) List(ctx context.Context) ([]ManagedRepo, error) {
	repos := make([]ManagedRepo, 0)
	err := filepath.WalkDir(m.baseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}

		ok, err := hasGitEntry(path)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}

		repo, err := m.inspectListedRepo(ctx, path)
		if err != nil {
			return err
		}
		repos = append(repos, repo)
		return filepath.SkipDir
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(repos, func(i, j int) bool {
		return repos[i].Key < repos[j].Key
	})
	return repos, nil
}

func (m *Manager) inspectListedRepo(ctx context.Context, root string) (ManagedRepo, error) {
	rel, err := filepath.Rel(m.baseDir, root)
	if err != nil {
		return ManagedRepo{}, err
	}

	key := filepath.ToSlash(rel)
	parts := strings.Split(key, "/")
	repo := ManagedRepo{
		Key:      key,
		Root:     root,
		Host:     firstOrEmpty(parts),
		RepoPath: key,
	}
	if len(parts) > 1 {
		repo.RepoPath = strings.Join(parts[1:], "/")
	}

	if data, err := m.git.Run(ctx, root, "remote", "get-url", "origin"); err == nil {
		repo.RemoteURL = strings.TrimSpace(string(data))
	}

	bareDir := filepath.Join(root, ".bare")
	if !isManagedRepoRoot(root, bareDir) {
		repo.Legacy = true
		return repo, nil
	}

	worktrees, remoteURL, err := m.inspectRepo(ctx, root, bareDir)
	if err != nil {
		return ManagedRepo{}, err
	}
	repo.Worktrees = worktrees
	if remoteURL != "" {
		repo.RemoteURL = remoteURL
	}
	return repo, nil
}

func (m *Manager) inspectRepo(ctx context.Context, root, bareDir string) ([]Worktree, string, error) {
	out, err := m.git.Run(ctx, root, "--git-dir", bareDir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, "", err
	}
	remoteURL := ""
	if data, err := m.git.Run(ctx, root, "--git-dir", bareDir, "remote", "get-url", "origin"); err == nil {
		remoteURL = strings.TrimSpace(string(data))
	}
	worktrees := parseWorktreeList(out)
	filtered := worktrees[:0]
	for _, wt := range worktrees {
		if wt.Path == bareDir {
			continue
		}
		filtered = append(filtered, wt)
	}
	return filtered, remoteURL, nil
}

func (m *Manager) defaultBranch(ctx context.Context, bareDir string) (string, error) {
	out, err := m.git.Run(ctx, m.baseDir, "--git-dir", bareDir, "symbolic-ref", "HEAD")
	if err == nil {
		ref := strings.TrimSpace(string(out))
		ref = strings.TrimPrefix(ref, "refs/heads/")
		if ref != "" {
			return ref, nil
		}
	}

	out, err = m.git.Run(ctx, m.baseDir, "--git-dir", bareDir, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err == nil {
		ref := strings.TrimSpace(string(out))
		ref = strings.TrimPrefix(ref, "refs/remotes/origin/")
		if ref != "" {
			return ref, nil
		}
	}

	return "", fmt.Errorf("cannot resolve default branch for %s", bareDir)
}

func (m *Manager) addNamedWorktree(ctx context.Context, bareDir, targetPath, worktreeName, defaultBranch string) error {
	localRef := "refs/heads/" + worktreeName
	if m.refExists(ctx, bareDir, localRef) {
		_, err := m.git.Run(ctx, m.baseDir, "--git-dir", bareDir, "worktree", "add", targetPath, worktreeName)
		return err
	}

	remoteRef := "refs/remotes/origin/" + worktreeName
	if m.refExists(ctx, bareDir, remoteRef) {
		_, err := m.git.Run(ctx, m.baseDir, "--git-dir", bareDir, "worktree", "add", "--track", "-b", worktreeName, targetPath, "origin/"+worktreeName)
		return err
	}

	_, err := m.git.Run(ctx, m.baseDir, "--git-dir", bareDir, "worktree", "add", "-b", worktreeName, targetPath, defaultBranch)
	return err
}

func (m *Manager) refExists(ctx context.Context, bareDir, ref string) bool {
	_, err := m.git.Run(ctx, m.baseDir, "--git-dir", bareDir, "show-ref", "--verify", "--quiet", ref)
	return err == nil
}

func (m *Manager) isManagedWorktree(ctx context.Context, bareDir, targetPath string) (bool, error) {
	out, err := m.git.Run(ctx, m.baseDir, "--git-dir", bareDir, "worktree", "list", "--porcelain")
	if err != nil {
		return false, err
	}

	targetPath = filepath.Clean(targetPath)
	for _, wt := range parseWorktreeList(out) {
		if filepath.Clean(wt.Path) == targetPath {
			return true, nil
		}
	}
	return false, nil
}

func (m *Manager) worktreeRoot(ctx context.Context, dir string) (string, error) {
	out, err := m.git.Run(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (m *Manager) remoteURL(ctx context.Context, dir string) (string, error) {
	out, err := m.git.Run(ctx, dir, "remote", "get-url", "origin")
	if err != nil {
		return "", fmt.Errorf("resolve origin remote: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (m *Manager) currentBranch(ctx context.Context, dir string) (string, error) {
	out, err := m.git.Run(ctx, dir, "branch", "--show-current")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (m *Manager) headCommit(ctx context.Context, dir string) (string, error) {
	out, err := m.git.Run(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (m *Manager) sourceDefaultBranch(ctx context.Context, dir string) (string, error) {
	out, err := m.git.Run(ctx, dir, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err == nil {
		ref := strings.TrimSpace(string(out))
		ref = strings.TrimPrefix(ref, "refs/remotes/origin/")
		if ref != "" {
			return ref, nil
		}
	}

	for _, name := range []string{"main", "master"} {
		if m.localBranchExists(ctx, dir, name) {
			return name, nil
		}
	}

	out, err = m.git.Run(ctx, dir, "symbolic-ref", "HEAD")
	if err != nil {
		return "", err
	}
	ref := strings.TrimSpace(string(out))
	ref = strings.TrimPrefix(ref, "refs/heads/")
	if ref == "" {
		return "", fmt.Errorf("cannot resolve default branch for %s", dir)
	}
	return ref, nil
}

func (m *Manager) localBranchExists(ctx context.Context, dir, branch string) bool {
	_, err := m.git.Run(ctx, dir, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

func ensureGitPointer(root, bareDir string) error {
	content := []byte("gitdir: " + bareDir + "\n")
	gitFile := filepath.Join(root, ".git")
	data, err := os.ReadFile(gitFile)
	if err == nil && bytes.Equal(data, content) {
		return nil
	}
	if err == nil && len(data) > 0 && !bytes.Equal(data, content) {
		return fmt.Errorf("%s exists and does not point to %s", gitFile, bareDir)
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(gitFile, content, 0o644)
}

func validateWorktreeName(name string) error {
	if name == "" {
		return fmt.Errorf("worktree name cannot be empty")
	}
	if strings.Contains(name, "/") {
		return fmt.Errorf("worktree name cannot contain '/'")
	}
	if name == ".bare" || strings.HasPrefix(name, ".") {
		return fmt.Errorf("invalid worktree name %q", name)
	}
	return nil
}

func defaultConvertWorktreeName(currentBranch, defaultBranch string) string {
	if currentBranch == "" || currentBranch == defaultBranch {
		return DefaultWorktreeName
	}
	return currentBranch
}

func hasGitEntry(path string) (bool, error) {
	_, err := os.Lstat(filepath.Join(path, ".git"))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func isManagedRepoRoot(root, bareDir string) bool {
	info, err := os.Stat(bareDir)
	if err != nil || !info.IsDir() {
		return false
	}

	data, err := os.ReadFile(filepath.Join(root, ".git"))
	if err != nil {
		return false
	}
	return bytes.Equal(data, []byte("gitdir: "+bareDir+"\n"))
}

func firstOrEmpty(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func syncWorktreeSnapshot(sourceRoot, targetPath string) error {
	entries, err := os.ReadDir(targetPath)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == ".git" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(targetPath, entry.Name())); err != nil {
			return err
		}
	}

	entries, err = os.ReadDir(sourceRoot)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == ".git" {
			continue
		}
		if err := copyTree(filepath.Join(sourceRoot, entry.Name()), filepath.Join(targetPath, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyTree(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		link, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(link, dst)
	}

	if info.IsDir() {
		if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyTree(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, info.Mode().Perm())
}

func parseWorktreeList(data []byte) []Worktree {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	out := make([]Worktree, 0)
	var current *Worktree

	flush := func() {
		if current == nil {
			return
		}
		current.Name = filepath.Base(current.Path)
		out = append(out, *current)
		current = nil
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			flush()
			continue
		}

		if strings.HasPrefix(line, "worktree ") {
			flush()
			current = &Worktree{Path: strings.TrimPrefix(line, "worktree ")}
			continue
		}
		if current == nil {
			continue
		}
		switch {
		case strings.HasPrefix(line, "branch refs/heads/"):
			current.Branch = strings.TrimPrefix(line, "branch refs/heads/")
		case line == "detached":
			current.Detached = true
		case strings.HasPrefix(line, "HEAD "):
			current.Head = strings.TrimPrefix(line, "HEAD ")
		}
	}
	flush()

	sort.Slice(out, func(i, j int) bool {
		return out[i].Path < out[j].Path
	})
	return out
}
