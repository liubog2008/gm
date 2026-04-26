# `gm feat` Design

## Goal

Add a `feat` command group for feature-branch worktrees in a managed repo.

Required subcommands:

```bash
gm feat add <name> [base]
gm feat sync [remote]
gm feat prune
```

Required behavior:

1. `feat add` creates a worktree and local branch with the same name.
2. `feat add` defaults to `main` as the base and accepts `main` or `upstream/main`.
3. Feature worktrees use a `.gm` YAML file to record sync state.
4. `feat sync` pushes the current feature branch to a remote and records that remote.
5. `feat prune` fetches remote state, then removes synced feature worktrees whose recorded remote branches have already been deleted.

## Terms

- `feature name`: The branch name and worktree directory name.
- `base`: The ref used as the start point for `feat add`.
- `repo root`: The managed repo container directory containing `.bare`, `.git`, `main/`, and named worktrees.
- `feature worktree`: A managed worktree with a `.gm` state file.
- `state file`: `${worktree path}/.gm`.

## CLI Shape

Add a new command group under the root command:

```text
gm
└── feat
    ├── add
    ├── sync
    └── prune
```

Examples:

```bash
gm feat add feat-login
gm feat add feat-login upstream/main
gm feat sync
gm feat sync upstream
gm feat prune
```

`feat` subcommands are current-directory based. They must be run from inside a managed repo worktree unless explicitly stated otherwise.

## `.gm` State File

Every feature worktree created by `gm feat add` must contain:

```text
${repo root}/${feature name}/.gm
```

Schema:

```yaml
status: local
remotes: []
```

Fields:

- `status`: whether the branch has been synchronized to at least one remote.
- `remotes`: remote names that have been synchronized, such as `origin` or `upstream`.

Status values:

- `local`: branch exists locally and is not recorded as synced.
- `synced`: branch has been pushed to at least one recorded remote.

The state file does not record a branch name. Feature branch names are always derived from the worktree directory name. For example, `${repo root}/feat-login` syncs branch `feat-login`.

`feat add` writes `remotes: []`. The first successful `feat sync [remote]` records the selected remote name.

Suggested Go model:

```go
type FeatureState struct {
    Status string `yaml:"status"`
    Remotes []string `yaml:"remotes"`
}
```

Use `gopkg.in/yaml.v3`, which is already a dependency.

## Managed Repo Detection

Resolution flow:

1. Run `git rev-parse --show-toplevel` from the current directory.
2. Walk upward from that path until a managed repo root is found.
3. A managed repo root is a directory with:
   - `.bare/`
   - `.git` containing `gitdir: <repo root>/.bare`
4. If no managed repo root is found, fail.

Example error:

```text
not inside a gm managed repo
```

The commands should accept being run from any managed worktree directory or nested directory inside a worktree.

## `gm feat add`

Purpose: create a new feature branch and worktree in the current managed repo, then print the new worktree path.

Interface:

```bash
gm feat add <name>
gm feat add <name> <base>
```

Argument validation:

- `<name>` is required.
- `<name>` uses the same validation as other worktree names:
  - non-empty
  - no `/`
  - not `.bare`
  - must not start with `.`
- `<base>` defaults to `main`.
- `<base>` must be exactly `main` or `upstream/main`.

Shell switching:

A child process cannot directly change its parent shell directory, so `feat add` prints the created worktree path to stdout. Users can wire that into a shell helper:

```bash
gmf() {
  local dir
  dir="$(gm feat add "$@")" || return 1
  cd "$dir"
}
```

Worktree creation:

Given:

- repo root: `${repo root}`
- bare repo: `${repo root}/.bare`
- feature name: `feat-login`
- target path: `${repo root}/feat-login`
- base: `main` or `upstream/main`

Preconditions:

1. `${repo root}/feat-login` must not already exist unless it is already a managed worktree for branch `feat-login`.
2. `refs/heads/feat-login` must not already exist unless the matching worktree already exists.
3. The selected base must exist.

Base resolution:

- `main` resolves to local branch `refs/heads/main`.
- `upstream/main` resolves to remote-tracking ref `refs/remotes/upstream/main`.

Do not silently fall back from one base to another. If the requested base does not exist, fail.

Suggested git checks:

```bash
git --git-dir <repo root>/.bare show-ref --verify --quiet refs/heads/<name>
git --git-dir <repo root>/.bare show-ref --verify --quiet refs/heads/main
git --git-dir <repo root>/.bare show-ref --verify --quiet refs/remotes/upstream/main
```

Suggested git creation:

```bash
git --git-dir <repo root>/.bare worktree add -b <name> <repo root>/<name> <base>
```

For `upstream/main`, pass the user-facing ref directly as the start point:

```bash
git --git-dir <repo root>/.bare worktree add -b feat-login <repo root>/feat-login upstream/main
```

Initial state file:

```yaml
status: local
remotes: []
```

Idempotency:

- If `${repo root}/${name}` exists and is a managed worktree for branch `<name>`, ensure `.gm` exists, then print the path.
- If the directory exists but is not a managed worktree, fail.
- If branch `<name>` exists without the matching worktree, fail.

## `gm feat sync`

Purpose: push the current feature branch to a remote branch with the same name, then record that remote in `.gm`.

Interface:

```bash
gm feat sync
gm feat sync <remote>
```

The command operates on the current worktree only.

Remote selection:

- If `<remote>` is omitted, use `origin`.
- If `<remote>` is provided, use that remote name.
- The remote must exist in git config.

Preconditions:

1. The current directory must be inside a managed worktree.
2. The current worktree must contain a `.gm` state file.
3. The current branch must match the worktree directory name.
4. The worktree directory name is the branch name used for sync.
5. The selected remote must exist.
6. The worktree must not have unresolved merge conflicts.

Recommended dirty-worktree policy:

- Allow untracked or modified files only if they are already committed before sync.
- Before syncing, run a status check and fail if there are staged, unstaged, or untracked changes.

Suggested checks:

```bash
git rev-parse --show-toplevel
git branch --show-current
git remote get-url <remote>
git status --porcelain
```

Suggested sync:

```bash
git push -u <remote> HEAD:<branch>
```

For `${repo root}/feat-login`, the branch name is `feat-login`:

```bash
git push -u origin HEAD:feat-login
```

On success:

1. Add the selected remote to `.gm remotes` if it is not already present.
2. Update `.gm status` to `synced`.

```yaml
status: synced
remotes:
  - origin
```

3. Print the synced remote branch:

```text
origin/feat-login
```

If sync fails, leave `.gm` unchanged.

## `gm feat prune`

Purpose: remove local feature worktrees that have already been synced and whose recorded remote branches have been deleted.

Interface:

```bash
gm feat prune
```

Scope:

- Operates on the current managed repo.
- Traverses all worktrees that belong to the current managed repo.
- Only considers directories that contain a `.gm` state file.
- Runs `git fetch --all --prune` before scanning so local remote-tracking refs reflect remote deletions.

Fetch:

```bash
git --git-dir <repo root>/.bare fetch --all --prune
```

Prune eligibility:

A feature worktree is eligible when all are true:

1. `.gm status` is `synced`.
2. `.gm remotes` is non-empty.
3. Every remote branch derived from `.gm remotes` and the worktree name no longer exists after fetch.
4. The directory is a managed git worktree for the repo's `.bare`.
5. The worktree has no staged, unstaged, or untracked changes.

Remote deletion check:

For this state:

```yaml
status: synced
remotes:
  - origin
  - upstream
```

and worktree path `${repo root}/feat-login`, check:

```bash
git --git-dir <repo root>/.bare show-ref --verify --quiet refs/remotes/origin/feat-login
git --git-dir <repo root>/.bare show-ref --verify --quiet refs/remotes/upstream/feat-login
```

If any recorded remote-tracking ref exists, keep the worktree. If all recorded remote-tracking refs are missing after fetch, the worktree can be pruned.

Current worktree handling:

- If `gm feat prune` is run from a worktree that is eligible for removal, change the process working directory to the repo root before removing it.
- Since the CLI cannot directly change its parent shell directory, return the repo root as the final directory for shell helpers.
- If the current worktree is not eligible for removal, keep it and do not change the final directory.

Prune action:

```bash
git --git-dir <repo root>/.bare worktree remove --force <worktree path>
```

`--force` is required because `.gm` is an untracked metadata file inside the worktree. Safety comes from the explicit dirty check that ignores only the root `.gm` file and skips worktrees with any other staged, unstaged, or untracked changes.

Branch handling:

- Initial implementation should remove only the worktree.
- Do not delete the local branch by default.
- A later `--delete-branch` flag can remove `refs/heads/<feature name>` after worktree removal.

Output:

- Print one removed worktree path per line.
- If the current worktree is removed, print the repo root last so a shell helper can `cd` there.
- If nothing is removed, print nothing and return success.

Suggested shell helper:

```bash
gmp() {
  local out
  out="$(gm feat prune "$@")" || return 1
  [ -z "$out" ] && return 0
  local last
  last="$(printf "%s\n" "$out" | tail -n 1)"
  [ -d "$last" ] && cd "$last"
}
```

Safety behavior:

- Skip dirty worktrees and report them on stderr.
- Skip worktrees whose recorded remote branch still exists on any recorded remote and do not report them as errors.
- Skip malformed `.gm` files and report them on stderr.
- Fail if fetch fails, because prune depends on fresh remote-tracking refs.
- Fail for repo-level errors that prevent scanning or calling git.

## Package Design

Keep the same package boundaries as existing code:

- `internal/cli`: Cobra command wiring and argument validation.
- `internal/repo`: managed repo detection, feature operations, `.gm` state handling.
- `internal/gitx`: no new abstraction required.

Suggested repo API:

```go
type FeatureAddOptions struct {
    Name string
    Base string
    StartDir string
}

type FeatureSyncOptions struct {
    StartDir string
    Remote string
}

type FeaturePruneOptions struct {
    StartDir string
    Stderr io.Writer
}

type FeaturePruneResult struct {
    RemovedPaths []string
    FinalDir string
}

func (m *Manager) AddFeatureWorktree(ctx context.Context, opts FeatureAddOptions) (string, error)
func (m *Manager) SyncFeatureWorktree(ctx context.Context, opts FeatureSyncOptions) (string, error)
func (m *Manager) PruneFeatureWorktrees(ctx context.Context, opts FeaturePruneOptions) (FeaturePruneResult, error)
```

`StartDir` can default to `os.Getwd()` in the CLI layer. Passing it explicitly keeps tests deterministic.

`FeaturePruneResult.FinalDir` is empty unless the current worktree was removed. When set, the CLI prints it after removed paths so shell helpers can move the parent shell to the repo root.

## Implementation Flow

`feat add`:

1. Parse `gm feat add <name> [base]`.
2. Default `base` to `main`.
3. Validate argument count and supported base values.
4. Resolve the managed repo root from the current directory.
5. Validate the base ref.
6. Check branch and target path conflicts.
7. Run `git worktree add -b`.
8. Write `${target path}/.gm`.
9. Print `${target path}`.

`feat sync`:

1. Resolve the current worktree root and managed repo root.
2. Read `${current worktree}/.gm`.
3. Default remote to `origin` if the user did not pass one.
4. Validate the selected remote exists.
5. Validate current branch matches the worktree directory name.
6. Derive the sync branch name from the worktree directory name.
7. Fail if `git status --porcelain` is non-empty.
8. Run `git push -u <remote> HEAD:<branch>`.
9. Append `<remote>` to `.gm remotes` if missing.
10. Update `.gm status` to `synced`.
11. Print `<remote>/<branch>`.

`feat prune`:

1. Resolve the current worktree root and managed repo root.
2. Run `git fetch --all --prune` for the repo's `.bare`.
3. List worktrees from `git worktree list --porcelain`.
4. Build the prune candidate list from every worktree with a `.gm` file:
   - parse state
   - require `status: synced`
   - require `.gm remotes` to be non-empty
   - require every recorded remote's derived ref to be missing under `refs/remotes/`
   - require clean `git status --porcelain`
5. If the current worktree is in the candidate list, `chdir` to repo root before removing any candidates.
6. Run `git worktree remove` for each candidate.
7. Print removed paths.
8. If the current worktree was removed, print the repo root last.

## Error Handling

Important errors:

1. not inside a managed repo
2. invalid feature name
3. unsupported base
4. missing requested base ref
5. target worktree path already exists but is not managed
6. feature branch already exists without matching worktree
7. missing or malformed `.gm`
8. selected remote does not exist
9. dirty worktree
10. fetch failure
11. git command failure

Example messages:

```text
unsupported base "origin/main"; expected "main" or "upstream/main"
base "upstream/main" does not exist in github.com/acme/demo
branch "feat-login" already exists without matching worktree
/base/github.com/acme/demo/feat-login exists but is not a gm worktree
current worktree is dirty; commit or stash changes before syncing
skip /base/github.com/acme/demo/feat-login: worktree is dirty
skip /base/github.com/acme/demo/feat-login: remote branch origin/feat-login still exists
```

## Tests

Unit tests:

1. `gm feat add` validates required args.
2. unsupported bases return usage errors.
3. feature state marshals to the expected YAML fields.
4. managed repo root detection works from `main/` and nested directories.
5. feature sync defaults remote to `origin`.
6. feature sync appends remote names without duplicates.

Integration tests using local temporary repos:

1. `feat add feat-login` creates branch, worktree, and `.gm`.
2. `feat add feat-login upstream/main` starts from `upstream/main`.
3. `feat add` rejects missing `upstream/main`.
4. `feat add` rejects an existing branch without a matching worktree.
5. repeating `feat add feat-login` returns the same path and preserves `.gm`.
6. `feat sync` pushes `HEAD` to `origin/<feature>`, records `origin`, and marks `.gm` synced.
7. `feat sync upstream` pushes `HEAD` to `upstream/<feature>` and records `upstream`.
8. `feat sync` fails on dirty worktree and leaves `.gm` unchanged.
9. `feat prune` fetches with prune before scanning.
10. `feat prune` removes synced clean feature worktrees whose recorded remote branches are all gone.
11. `feat prune` skips local, dirty, malformed, and still-remote-backed worktrees.
12. `feat prune` removes the current worktree when eligible and returns the repo root as the final directory.

Use local bare repos and local remotes only. Do not require network access.

## Out Of Scope

Initial implementation should not include:

- automatic PR creation
- feature branch delete after prune
- arbitrary base branches
- force push
- remote branch deletion
- interactive prune selection
