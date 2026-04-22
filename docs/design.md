# gm Design Doc

## Goal

Implement a Go CLI named `gm` to manage git repositories under a single base directory.

Required commands:

- `gm get`
- `gm list`
- `gm cd`

Required behaviors:

1. Repository paths are derived from repo URLs. For example, `https://github.com/liubog2008/gm` maps to `${base}/github.com/liubog2008/gm`.
2. Each managed repo uses `.bare` for the bare repository, `.git` points to `.bare`, and each worktree uses its worktree name as the directory name.
3. `gm cd` supports fuzzy matching for repos and worktrees, such as `gm cd gm` and `gm cd gm:main`.

## Terms

- `base`: Root directory containing all managed repositories.
- `repo url`: Remote URL such as `https://github.com/liubog2008/gm`.
- `repo root`: Local root path mapped from the repo URL, such as `${base}/github.com/liubog2008/gm`.
- `bare repo dir`: `${repo root}/.bare`
- `main worktree`: `${repo root}/main`
- `named worktree`: `${repo root}/feature-x`

## Directory Layout

Example for `https://github.com/liubog2008/gm`:

```text
${base}/github.com/liubog2008/gm/
├── .bare/
├── .git
├── main/
├── feature-a/
└── feature-b/
```

Rules:

- `.bare` stores the bare repository.
- `${repo root}/.git` is a text file pointing to `.bare`:

```text
gitdir: /abs/path/to/${base}/github.com/liubog2008/gm/.bare
```

- `main/`, `feature-a/`, and other directories are worktrees created from the bare repo.
- `repo root` is a container directory and is not itself a worktree.

## URL Mapping

Supported URL forms:

- `https://github.com/liubog2008/gm`
- `https://github.com/liubog2008/gm.git`
- `git@github.com:liubog2008/gm.git`
- `ssh://git@github.com/liubog2008/gm.git`

Normalization rules:

1. Extract host.
2. Extract path.
3. Remove trailing `.git`.
4. Validate that the path contains at least `owner/repo`.
5. Join as `${base}/${host}/${repoPath}`.

Example:

- host: `github.com`
- repo path: `liubog2008/gm`
- repo root: `${base}/github.com/liubog2008/gm`

Suggested model:

```go
type RepoLocator struct {
    Host string
    Path string
    Root string
}
```

## Command Design

### `gm get`

Purpose: ensure the repo exists locally and ensure a target worktree exists.

Interface:

```bash
gm get <repo-url>
gm get <repo-url> <worktree>
```

Semantics:

- `gm get <repo-url>` creates or updates the repo and ensures `main` exists.
- `gm get <repo-url> <worktree>` creates or updates the repo and ensures `<worktree>` exists.

Behavior:

1. Resolve `repo root` from the URL.
2. Create parent directories if needed.
3. If `.bare` does not exist, run a bare clone.
4. If `.git` does not exist, write a pointer to `.bare`.
5. Fetch remote updates.
6. Resolve the default branch from `origin/HEAD`.
7. If the target worktree does not exist:
   - For `main`, create it from the default branch.
   - For other names:
     - prefer an existing local branch
     - otherwise prefer an existing remote branch
     - otherwise create a new branch from the default branch
8. Print the absolute worktree path.

Example:

```bash
gm get https://github.com/liubog2008/gm
gm get https://github.com/liubog2008/gm feat-x
```

### `gm list`

Purpose: list all managed repos and their worktrees.

Interface:

```bash
gm list
gm list --long
gm list --json
```

Initial version may implement only the default text output.

Suggested default output:

```text
github.com/liubog2008/gm
  main      branch=main
  feat-x    branch=feat-x

github.com/foo/bar
  main      branch=master
```

Implementation:

- Scan `${base}` for directories containing `.bare`.
- For each repo, use `git worktree list --porcelain`.
- Gather remote URL and worktree details.

Suggested model:

```go
type ManagedRepo struct {
    RepoKey      string
    Root         string
    RemoteURL    string
    DefaultRef   string
    Worktrees    []WorktreeInfo
}

type WorktreeInfo struct {
    Name         string
    Path         string
    Branch       string
    Detached     bool
    Head         string
}
```

### `gm cd`

Purpose: return the path that the shell should `cd` into.

Interface:

```bash
gm cd <query>
```

Behavior:

- Print a single absolute path on stdout.
- Return non-zero on no match or ambiguous match.
- If a repo matches, return its default worktree path, preferably `main`.
- If a repo/worktree selector matches, return that worktree path.

Examples:

```bash
gm cd gm
gm cd gm:main
gm cd liubog2008/gm
gm cd github.com/liubog2008/gm:feat-x
```

Expected outputs:

- `gm cd gm` -> `${base}/github.com/liubog2008/gm/main`
- `gm cd gm:main` -> `${base}/github.com/liubog2008/gm/main`
- `gm cd liubog2008/gm` -> `${base}/github.com/liubog2008/gm/main`
- `gm cd github.com/liubog2008/gm:feat-x` -> `${base}/github.com/liubog2008/gm/feat-x`

## `gm cd` Matching Rules

Supported query shapes:

1. Repo-only query:
   - `gm`
   - `liubog2008/gm`
   - `github.com/liubog2008/gm`
2. Repo plus worktree query:
   - `gm:main`
   - `liubog2008/gm:feat-x`
   - `github.com/liubog2008/gm:main`

Matching targets:

- repo key: `github.com/liubog2008/gm`
- short repo key: `liubog2008/gm`
- repo name: `gm`
- worktree selectors built from repo key plus worktree name

Priority:

1. exact worktree selector match
2. exact repo selector match
3. exact short `repo/worktree` match
4. exact repo name match
5. suffix match
6. substring match

Conflict handling:

- zero matches: return an error
- one match: return the path
- multiple matches:
  - if one candidate has a strictly better score, use it
  - otherwise return an ambiguity error with candidate labels

Suggested score shape:

```go
type MatchScore struct {
    ExactLevel int
    TargetType int
    LengthBias int
}
```

## Git Execution Strategy

Use the system `git` command instead of reimplementing git logic.

Suggested abstraction:

```go
type GitRunner interface {
    Run(ctx context.Context, dir string, args ...string) ([]byte, error)
}
```

Key commands:

Initialize bare repo:

```bash
git clone --bare <repo-url> <repo-root>/.bare
```

Fetch:

```bash
git --git-dir=.bare fetch --all --prune
```

Resolve default branch:

```bash
git --git-dir=.bare symbolic-ref refs/remotes/origin/HEAD
```

List worktrees:

```bash
git --git-dir=.bare worktree list --porcelain
```

Create main worktree:

```bash
git --git-dir=.bare worktree add <repo-root>/main <default-branch>
```

Create named worktree:

- local branch exists: `worktree add <path> <branch>`
- remote branch exists: `worktree add <path> --track -b <branch> origin/<branch>`
- otherwise: `worktree add -b <branch> <path> <default-branch>`

## Config

Initial version should keep config small.

Suggested sources in order:

1. CLI flag
2. YAML config file

Supported settings:

```go
type Config struct {
    BaseDir string
}
```

Suggested inputs:

- `--base`
- `--config`
- `$HOME/.config/gm/config.yaml`

Suggested YAML:

```yaml
base_dir: /path/to/repos
```

If none of them is provided, fail fast instead of guessing a path.

## CLI Shape

```text
gm
├── get
├── list
└── cd
```

Examples:

```bash
gm get https://github.com/liubog2008/gm
gm get https://github.com/liubog2008/gm feat-x
gm list
gm cd gm
gm cd gm/main
```

Exit codes:

- `0`: success
- `1`: business error
- `2`: usage error

## Package Layout

Suggested package structure:

```text
cmd/gm/
internal/config/
internal/repo/
internal/gitx/
internal/index/
internal/match/
internal/cli/
```

Responsibilities:

- `config`: resolve base dir
- `repo`: repo layout and URL mapping
- `gitx`: git command wrapper
- `index`: scan managed repos
- `match`: fuzzy matching for `gm cd`
- `cli`: command entrypoints

Initial implementation may merge some of these packages to stay small.

## Indexing and Performance

For v1, avoid a persistent index.

Behavior:

- scan `${base}` on demand
- treat any directory containing `.bare` as a managed repo
- parse worktree metadata from git each time

Why:

- simple
- consistent
- usually fast enough for a local workstation

Persistent cache such as `${base}/.gm/index.json` can be added later if needed.

## Error Handling

Important error classes:

1. invalid repo URL
2. existing repo directory with invalid structure
3. existing worktree directory not managed by the bare repo
4. `gm cd` no match
5. `gm cd` ambiguous match
6. git command failure

Error messages should be directly actionable.

Examples:

```text
ambiguous query "gm", candidates:
- github.com/liubog2008/gm
- github.com/acme/gm
```

```text
no worktree "feat-x" in repo github.com/liubog2008/gm
```

## Shell Integration

`gm cd` cannot directly change the parent shell directory, so it should print the target path.

Example shell helper:

```bash
gmc() {
  local dir
  dir="$(gm cd "$@")" || return 1
  cd "$dir"
}
```

Optional future improvement:

- `gm init zsh`
- `gm init bash`

These commands could emit a shell function for direct integration.

## Testing Strategy

Unit tests:

1. URL normalization
2. URL to local path mapping
3. worktree name validation
4. `gm cd` matching and ambiguity handling

Integration tests:

1. initialize a bare repo from a local temp remote
2. `gm get` is idempotent
3. create a named worktree
4. `gm list` shows repo and worktrees
5. `gm cd gm`
6. `gm cd gm/main`
7. ambiguous repo names produce an error

Use local temporary repos for tests instead of network dependencies.

## Initial Scope

Version 1 should include:

1. URL normalization for `https` and `git@host:path.git`
2. `gm get`
3. `gm list`
4. `gm cd`
5. `--base` and YAML config
6. shell helper documented externally

Version 1 should not include:

- persistent index
- delete repo/worktree
- rename worktree
- interactive selection
- shell completion
- multi-remote management

## Example Scenario

Run:

```bash
gm get https://github.com/liubog2008/gm
```

Result:

```text
/base/github.com/liubog2008/gm/
  .bare/
  .git
  main/
```

Run:

```bash
gm get https://github.com/liubog2008/gm feat-login
```

Result:

```text
/base/github.com/liubog2008/gm/
  .bare/
  .git
  main/
  feat-login/
```

Run:

```bash
gm cd gm
```

Output:

```text
/base/github.com/liubog2008/gm/main
```

Run:

```bash
gm cd gm/feat-login
```

Output:

```text
/base/github.com/liubog2008/gm/feat-login
```

## Implementation Order

1. config and URL normalization
2. repo layout and `.bare` / `.git` handling
3. `gm get`
4. `gm list`
5. `gm cd`
6. shell helper docs
7. unit and integration tests
