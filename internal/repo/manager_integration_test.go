package repo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/liubog2008/gm/internal/gitx"
)

func TestFeatureAddCreatesWorktreeAndState(t *testing.T) {
	ctx := context.Background()
	env := setupManagedRepo(t, ctx)

	manager := NewManager(env.base, gitx.CommandRunner{})
	got, err := manager.AddFeatureWorktree(ctx, FeatureAddOptions{
		Name:     "feat-login",
		StartDir: env.mainWorktree,
	})
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(env.root, "feat-login")
	if got != want {
		t.Fatalf("AddFeatureWorktree() = %q, want %q", got, want)
	}
	branch := strings.TrimSpace(gitOutput(t, ctx, got, "branch", "--show-current"))
	if branch != "feat-login" {
		t.Fatalf("branch = %q, want feat-login", branch)
	}
	checkFile(t, filepath.Join(got, ".gm"), "status: local\nremotes: []\n")
}

func TestFeatureSyncDefaultsOriginAndRecordsRemote(t *testing.T) {
	ctx := context.Background()
	env := setupManagedRepo(t, ctx)
	manager := NewManager(env.base, gitx.CommandRunner{})
	worktree, err := manager.AddFeatureWorktree(ctx, FeatureAddOptions{
		Name:     "feat-login",
		StartDir: env.mainWorktree,
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := manager.SyncFeatureWorktree(ctx, FeatureSyncOptions{StartDir: worktree})
	if err != nil {
		t.Fatal(err)
	}
	if got != "origin/feat-login" {
		t.Fatalf("SyncFeatureWorktree() = %q, want origin/feat-login", got)
	}
	if !refExistsInGit(t, ctx, env.originRemote, "refs/heads/feat-login") {
		t.Fatalf("origin remote branch was not created")
	}
	checkFile(t, filepath.Join(worktree, ".gm"), "status: synced\nremotes:\n    - origin\n")
}

func TestFeatureSyncUsesRequestedRemote(t *testing.T) {
	ctx := context.Background()
	env := setupManagedRepo(t, ctx)
	upstreamRemote := filepath.Join(env.tmp, "upstream.git")
	mustGit(t, ctx, env.tmp, "init", "--bare", upstreamRemote)
	mustGit(t, ctx, env.mainWorktree, "remote", "add", "upstream", upstreamRemote)

	manager := NewManager(env.base, gitx.CommandRunner{})
	worktree, err := manager.AddFeatureWorktree(ctx, FeatureAddOptions{
		Name:     "feat-login",
		StartDir: env.mainWorktree,
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := manager.SyncFeatureWorktree(ctx, FeatureSyncOptions{StartDir: worktree, Remote: "upstream"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "upstream/feat-login" {
		t.Fatalf("SyncFeatureWorktree() = %q, want upstream/feat-login", got)
	}
	if !refExistsInGit(t, ctx, upstreamRemote, "refs/heads/feat-login") {
		t.Fatalf("upstream remote branch was not created")
	}
	checkFile(t, filepath.Join(worktree, ".gm"), "status: synced\nremotes:\n    - upstream\n")
}

func TestFeaturePruneRemovesSyncedWorktreeWhoseRemoteBranchIsGone(t *testing.T) {
	ctx := context.Background()
	env := setupManagedRepo(t, ctx)
	manager := NewManager(env.base, gitx.CommandRunner{})
	worktree, err := manager.AddFeatureWorktree(ctx, FeatureAddOptions{
		Name:     "feat-login",
		StartDir: env.mainWorktree,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SyncFeatureWorktree(ctx, FeatureSyncOptions{StartDir: worktree}); err != nil {
		t.Fatal(err)
	}
	mustGit(t, ctx, env.originRemote, "update-ref", "-d", "refs/heads/feat-login")

	result, err := manager.PruneFeatureWorktrees(ctx, FeaturePruneOptions{StartDir: env.mainWorktree})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.RemovedPaths) != 1 || result.RemovedPaths[0] != worktree {
		t.Fatalf("RemovedPaths = %#v, want [%q]", result.RemovedPaths, worktree)
	}
	if result.FinalDir != "" {
		t.Fatalf("FinalDir = %q, want empty", result.FinalDir)
	}
	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Fatalf("worktree still exists or unexpected stat error: %v", err)
	}
}

func TestFeaturePruneCurrentWorktreeReturnsRepoRoot(t *testing.T) {
	ctx := context.Background()
	env := setupManagedRepo(t, ctx)
	manager := NewManager(env.base, gitx.CommandRunner{})
	worktree, err := manager.AddFeatureWorktree(ctx, FeatureAddOptions{
		Name:     "feat-login",
		StartDir: env.mainWorktree,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SyncFeatureWorktree(ctx, FeatureSyncOptions{StartDir: worktree}); err != nil {
		t.Fatal(err)
	}
	mustGit(t, ctx, env.originRemote, "update-ref", "-d", "refs/heads/feat-login")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatal(err)
		}
	}()

	result, err := manager.PruneFeatureWorktrees(ctx, FeaturePruneOptions{StartDir: worktree})
	if err != nil {
		t.Fatal(err)
	}
	if result.FinalDir != env.root {
		t.Fatalf("FinalDir = %q, want %q", result.FinalDir, env.root)
	}
	gotWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if gotWD != env.root {
		t.Fatalf("cwd = %q, want %q", gotWD, env.root)
	}
}

func TestManagerListFromGitWorktree(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	source := filepath.Join(tmp, "source")
	base := filepath.Join(tmp, "base")
	root := filepath.Join(base, "github.com", "acme", "demo")
	bare := filepath.Join(root, ".bare")
	mainWorktree := filepath.Join(root, "main")

	mustGit(t, ctx, tmp, "init", source)
	mustGit(t, ctx, source, "config", "user.email", "test@example.com")
	mustGit(t, ctx, source, "config", "user.name", "Test User")
	mustGit(t, ctx, source, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("# demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, ctx, source, "add", "README.md")
	mustGit(t, ctx, source, "commit", "-m", "initial")
	branch := strings.TrimSpace(gitOutput(t, ctx, source, "branch", "--show-current"))

	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	mustGit(t, ctx, tmp, "clone", "--bare", source, bare)
	if err := ensureGitPointer(root, bare); err != nil {
		t.Fatal(err)
	}
	mustGit(t, ctx, root, "--git-dir", bare, "worktree", "add", mainWorktree, branch)

	manager := NewManager(base, gitx.CommandRunner{})
	repos, err := manager.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Fatalf("len(repos) = %d, want 1", len(repos))
	}
	if repos[0].Key != "github.com/acme/demo" {
		t.Fatalf("repo key = %q", repos[0].Key)
	}
	if len(repos[0].Worktrees) != 1 {
		t.Fatalf("len(worktrees) = %d, want 1", len(repos[0].Worktrees))
	}
	if repos[0].Worktrees[0].Name != "main" {
		t.Fatalf("worktree name = %q, want main", repos[0].Worktrees[0].Name)
	}
	if repos[0].Worktrees[0].Branch != branch {
		t.Fatalf("worktree branch = %q, want %q", repos[0].Worktrees[0].Branch, branch)
	}
}

type managedRepoTestEnv struct {
	tmp          string
	base         string
	root         string
	bare         string
	mainWorktree string
	originRemote string
}

func setupManagedRepo(t *testing.T, ctx context.Context) managedRepoTestEnv {
	t.Helper()
	tmp := t.TempDir()
	originRemote := filepath.Join(tmp, "origin.git")
	source := filepath.Join(tmp, "source")
	base := filepath.Join(tmp, "base")
	root := filepath.Join(base, "github.com", "acme", "demo")
	bare := filepath.Join(root, ".bare")
	mainWorktree := filepath.Join(root, "main")

	mustGit(t, ctx, tmp, "init", "--bare", originRemote)
	mustGit(t, ctx, tmp, "clone", originRemote, source)
	mustGit(t, ctx, source, "config", "user.email", "test@example.com")
	mustGit(t, ctx, source, "config", "user.name", "Test User")
	mustGit(t, ctx, source, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("# demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, ctx, source, "add", "README.md")
	mustGit(t, ctx, source, "commit", "-m", "initial")
	mustGit(t, ctx, source, "branch", "-M", "main")
	mustGit(t, ctx, source, "push", "-u", "origin", "main")
	mustGit(t, ctx, originRemote, "symbolic-ref", "HEAD", "refs/heads/main")

	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	mustGit(t, ctx, tmp, "clone", "--bare", originRemote, bare)
	if err := ensureGitPointer(root, bare); err != nil {
		t.Fatal(err)
	}
	mustGit(t, ctx, root, "--git-dir", bare, "worktree", "add", mainWorktree, "main")

	return managedRepoTestEnv{
		tmp:          tmp,
		base:         base,
		root:         root,
		bare:         bare,
		mainWorktree: mainWorktree,
		originRemote: originRemote,
	}
}

func TestIsManagedWorktreeRejectsPlainDirectory(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	source := filepath.Join(tmp, "source")
	base := filepath.Join(tmp, "base")
	root := filepath.Join(base, "github.com", "acme", "demo")
	bare := filepath.Join(root, ".bare")
	plainDir := filepath.Join(root, "main")

	mustGit(t, ctx, tmp, "init", source)
	mustGit(t, ctx, source, "config", "user.email", "test@example.com")
	mustGit(t, ctx, source, "config", "user.name", "Test User")
	mustGit(t, ctx, source, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("# demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, ctx, source, "add", "README.md")
	mustGit(t, ctx, source, "commit", "-m", "initial")

	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	mustGit(t, ctx, tmp, "clone", "--bare", source, bare)
	if err := os.MkdirAll(plainDir, 0o755); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(base, gitx.CommandRunner{})
	ok, err := manager.isManagedWorktree(ctx, bare, plainDir)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatalf("isManagedWorktree() = true, want false")
	}
}

func TestConvertRepoDefaultBranchToMainWorktree(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	remote := filepath.Join(tmp, "remote.git")
	source := filepath.Join(tmp, "source")
	base := filepath.Join(tmp, "base")
	remoteURL := "https://github.com/acme/demo.git"

	mustGit(t, ctx, tmp, "init", "--bare", remote)
	mustGit(t, ctx, tmp, "clone", remote, source)
	mustGit(t, ctx, source, "config", "user.email", "test@example.com")
	mustGit(t, ctx, source, "config", "user.name", "Test User")
	mustGit(t, ctx, source, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("# demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, ctx, source, "add", "README.md")
	mustGit(t, ctx, source, "commit", "-m", "initial")
	mustGit(t, ctx, source, "branch", "-M", "main")
	mustGit(t, ctx, source, "push", "-u", "origin", "main")
	mustGit(t, ctx, remote, "symbolic-ref", "HEAD", "refs/heads/main")
	mustGit(t, ctx, source, "remote", "set-url", "origin", remoteURL)
	if err := os.WriteFile(filepath.Join(source, "notes.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(base, gitx.CommandRunner{})
	got, err := manager.ConvertRepo(ctx, source, "")
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(base, "github.com", "acme", "demo", "main")
	if got != want {
		t.Fatalf("ConvertRepo() path = %q, want %q", got, want)
	}

	checkFile(t, filepath.Join(got, "README.md"), "# demo\n")
	checkFile(t, filepath.Join(got, "notes.txt"), "dirty\n")
	checkFile(t, filepath.Join(base, "github.com", "acme", "demo", ".git"), "gitdir: "+filepath.Join(base, "github.com", "acme", "demo", ".bare")+"\n")

	branch := strings.TrimSpace(gitOutput(t, ctx, got, "branch", "--show-current"))
	if branch != "main" {
		t.Fatalf("branch = %q, want main", branch)
	}
}

func TestConvertRepoFeatureBranchUsesBranchNameByDefault(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	remote := filepath.Join(tmp, "remote.git")
	source := filepath.Join(tmp, "source")
	base := filepath.Join(tmp, "base")
	remoteURL := "https://github.com/acme/demo.git"

	mustGit(t, ctx, tmp, "init", "--bare", remote)
	mustGit(t, ctx, tmp, "clone", remote, source)
	mustGit(t, ctx, source, "config", "user.email", "test@example.com")
	mustGit(t, ctx, source, "config", "user.name", "Test User")
	mustGit(t, ctx, source, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("# demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, ctx, source, "add", "README.md")
	mustGit(t, ctx, source, "commit", "-m", "initial")
	mustGit(t, ctx, source, "branch", "-M", "main")
	mustGit(t, ctx, source, "push", "-u", "origin", "main")
	mustGit(t, ctx, remote, "symbolic-ref", "HEAD", "refs/heads/main")
	mustGit(t, ctx, source, "remote", "set-url", "origin", remoteURL)
	mustGit(t, ctx, source, "checkout", "-b", "feature-x")
	if err := os.WriteFile(filepath.Join(source, "feature.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, ctx, source, "add", "feature.txt")
	mustGit(t, ctx, source, "commit", "-m", "feature")

	manager := NewManager(base, gitx.CommandRunner{})
	got, err := manager.ConvertRepo(ctx, source, "")
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(base, "github.com", "acme", "demo", "feature-x")
	if got != want {
		t.Fatalf("ConvertRepo() path = %q, want %q", got, want)
	}

	branch := strings.TrimSpace(gitOutput(t, ctx, got, "branch", "--show-current"))
	if branch != "feature-x" {
		t.Fatalf("branch = %q, want feature-x", branch)
	}
	checkFile(t, filepath.Join(got, "feature.txt"), "hello\n")
}

func TestListIncludesLegacyRepoAndSkipsNestedRepos(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	base := filepath.Join(tmp, "base")
	legacyRoot := filepath.Join(base, "github.com", "acme", "legacy")
	nestedRoot := filepath.Join(legacyRoot, "nested")

	if err := os.MkdirAll(legacyRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	mustGit(t, ctx, legacyRoot, "init")
	mustGit(t, ctx, legacyRoot, "config", "user.email", "test@example.com")
	mustGit(t, ctx, legacyRoot, "config", "user.name", "Test User")

	if err := os.MkdirAll(nestedRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	mustGit(t, ctx, nestedRoot, "init")

	manager := NewManager(base, gitx.CommandRunner{})
	repos, err := manager.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Fatalf("len(repos) = %d, want 1", len(repos))
	}
	if repos[0].Key != "github.com/acme/legacy" {
		t.Fatalf("repo key = %q, want github.com/acme/legacy", repos[0].Key)
	}
	if !repos[0].Legacy {
		t.Fatalf("Legacy = false, want true")
	}
}

func TestListTreatsManagedRootAsBoundary(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	source := filepath.Join(tmp, "source")
	base := filepath.Join(tmp, "base")
	root := filepath.Join(base, "github.com", "acme", "demo")
	bare := filepath.Join(root, ".bare")
	mainWorktree := filepath.Join(root, "main")
	nestedLegacy := filepath.Join(mainWorktree, "nested")

	mustGit(t, ctx, tmp, "init", source)
	mustGit(t, ctx, source, "config", "user.email", "test@example.com")
	mustGit(t, ctx, source, "config", "user.name", "Test User")
	mustGit(t, ctx, source, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("# demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, ctx, source, "add", "README.md")
	mustGit(t, ctx, source, "commit", "-m", "initial")
	branch := strings.TrimSpace(gitOutput(t, ctx, source, "branch", "--show-current"))

	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	mustGit(t, ctx, tmp, "clone", "--bare", source, bare)
	if err := ensureGitPointer(root, bare); err != nil {
		t.Fatal(err)
	}
	mustGit(t, ctx, root, "--git-dir", bare, "worktree", "add", mainWorktree, branch)

	if err := os.MkdirAll(nestedLegacy, 0o755); err != nil {
		t.Fatal(err)
	}
	mustGit(t, ctx, nestedLegacy, "init")

	manager := NewManager(base, gitx.CommandRunner{})
	repos, err := manager.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Fatalf("len(repos) = %d, want 1", len(repos))
	}
	if repos[0].Key != "github.com/acme/demo" {
		t.Fatalf("repo key = %q, want github.com/acme/demo", repos[0].Key)
	}
	if repos[0].Legacy {
		t.Fatalf("Legacy = true, want false")
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

func gitOutput(t *testing.T, ctx context.Context, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func checkFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("file %s = %q, want %q", path, data, want)
	}
}

func refExistsInGit(t *testing.T, ctx context.Context, dir, ref string) bool {
	t.Helper()
	cmd := exec.CommandContext(ctx, "git", "show-ref", "--verify", "--quiet", ref)
	cmd.Dir = dir
	return cmd.Run() == nil
}
