# gm

`gm` is a Go CLI for managing Git repositories and worktrees under a single base directory.

## Features

- Clone a repository into a managed layout with a bare repo plus named worktrees
- Reuse or create worktrees with `gm get`
- Convert an existing local repository into the managed layout with `gm convert`
- Fuzzy-search repos and worktrees from the command line or an interactive TUI
- Track recent selections to improve navigation ranking

## Install

```bash
go install github.com/liubog2008/gm/cmd/gm@latest
```

Or build from source:

```bash
go build ./cmd/gm
```

## Configuration

`gm` needs a base directory for managed repositories.

You can provide it either by flag:

```bash
gm --base ~/src
```

Or by config file at `~/.config/gm/config.yaml`:

```yaml
base_dir: ~/src
```

The config also accepts `base` as an alias for `base_dir`.

## Usage

Open the selector for managed repos and worktrees:

```bash
gm
```

To let `gm` change your current shell directory after a selection, add shell integration:

```bash
eval "$(gm init zsh)"
```

For bash:

```bash
eval "$(gm init bash)"
```

After that, `gm`, `gm get ...`, and `gm convert ...` will `cd` into the selected or created directory in your current shell.

Filter results directly:

```bash
gm -f gm
gm -f github.com/liubog2008/gm:main
```

Print all matches instead of launching the TUI:

```bash
gm -o
gm -o -f gm
gm -o -r
gm -o -w
```

Ensure a repo exists and get a worktree path:

```bash
gm get https://github.com/liubog2008/gm
gm get https://github.com/liubog2008/gm feature-x
```

Convert an existing repo into the managed layout:

```bash
gm convert /path/to/repo
gm convert /path/to/repo feature-x
```

## Managed Layout

For a repo such as `https://github.com/liubog2008/gm`, the managed layout looks like:

```text
${base}/github.com/liubog2008/gm/
├── .bare/
├── .git
├── main/
└── feature-x/
```

- `.bare` stores the bare repository
- `.git` points at `.bare`
- `main/` and other directories are worktrees

## Development

Run tests:

```bash
GOCACHE=/tmp/gocache go test ./...
```

Build the CLI:

```bash
GOCACHE=/tmp/gocache go build ./cmd/gm
```

## License

Apache License 2.0. See `LICENSE`.
