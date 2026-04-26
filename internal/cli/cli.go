package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/liubog2008/gm/internal/config"
	"github.com/liubog2008/gm/internal/gitx"
	"github.com/liubog2008/gm/internal/repo"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var errUsage = errors.New("usage")

const (
	shellIntegrationEnv = "GM_SHELL_INTEGRATION"
	shellCDPrefix       = "__gm_cd__:"
)

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	cmd := newRootCommand(stdout, stderr)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(ctx)
	if err == nil {
		return nil
	}
	if errors.Is(err, errUsage) {
		return err
	}
	if strings.HasPrefix(err.Error(), "unknown command ") {
		_ = cmd.Usage()
		name := strings.TrimPrefix(err.Error(), "unknown command ")
		if idx := strings.Index(name, " for "); idx >= 0 {
			name = name[:idx]
		}
		name = strings.Trim(name, `"`)
		return fmt.Errorf("%w: unknown subcommand %q", errUsage, name)
	}
	return err
}

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, errUsage) {
		return 2
	}
	return 1
}

func newRootCommand(stdout, stderr io.Writer) *cobra.Command {
	var baseDir string
	var configPath string
	var manager *repo.Manager
	var root *cobra.Command

	root = &cobra.Command{
		Use:           "gm",
		Short:         "Manage git repos and worktrees under a base directory",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageError(cmd, fmt.Sprintf("unknown subcommand %q", args[0]))
			}
			cfg, err := config.Load(baseDir, configPath)
			if err != nil {
				return err
			}
			manager = repo.NewManager(cfg.BaseDir, gitx.CommandRunner{})
			filter, _ := cmd.Flags().GetString("filter")
			outputAll, _ := cmd.Flags().GetBool("output-all")
			onlyRepo, _ := cmd.Flags().GetBool("repo")
			onlyWorktree, _ := cmd.Flags().GetBool("worktree")
			return runNavigate(cmd.Context(), manager, stdout, configPath, navigateOptions{
				filter:       filter,
				outputAll:    outputAll,
				onlyRepo:     onlyRepo,
				onlyWorktree: onlyWorktree,
			})
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd == root {
				return nil
			}

			cfg, err := config.Load(baseDir, configPath)
			if err != nil {
				return err
			}

			runner := gitx.CommandRunner{}
			if cmd.Name() == "get" || commandUnder(cmd, "feat") {
				runner = gitx.CommandRunner{
					Stdout:          stderr,
					Stderr:          stderr,
					StreamGitOutput: true,
				}
			}

			manager = repo.NewManager(cfg.BaseDir, runner)
			return nil
		},
	}

	root.SetOut(stderr)
	root.SetErr(stderr)
	configureBaseFlag(root.PersistentFlags(), &baseDir)
	configureConfigFlag(root.PersistentFlags(), &configPath)
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), err)
		_ = cmd.Usage()
		return fmt.Errorf("%w: %v", errUsage, err)
	})

	root.Flags().StringP("filter", "f", "", "filter repos and worktrees")
	root.Flags().BoolP("output-all", "o", false, "print all matching entries")
	root.Flags().BoolP("repo", "r", false, "show only repo directories")
	root.Flags().BoolP("worktree", "w", false, "show only worktree directories")
	root.AddCommand(newGetCommand(&manager, stdout))
	root.AddCommand(newConvertCommand(&manager, stdout))
	root.AddCommand(newInitCommand(stdout))
	root.AddCommand(newFeatCommand(&manager, stdout, stderr))

	return root
}

func newGetCommand(manager **repo.Manager, stdout io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <repo-url> [worktree]",
		Short: "Clone or reuse a managed repo worktree",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 || len(args) > 2 {
				return fmt.Errorf("%w: get requires <repo-url> [worktree]", errUsage)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			worktreeName := repo.DefaultWorktreeName
			if len(args) == 2 {
				worktreeName = args[1]
			}

			path, err := (*manager).EnsureRepo(cmd.Context(), args[0], worktreeName)
			if err != nil {
				return err
			}

			return printDir(stdout, path)
		},
	}

	cmd.Flags().SetInterspersed(false)
	return cmd
}

func newConvertCommand(manager **repo.Manager, stdout io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "convert <repo-path> [worktree]",
		Short: "Convert an existing git repo into the managed layout",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 || len(args) > 2 {
				return fmt.Errorf("%w: convert requires <repo-path> [worktree]", errUsage)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			worktreeName := ""
			if len(args) == 2 {
				worktreeName = args[1]
			}

			path, err := (*manager).ConvertRepo(cmd.Context(), args[0], worktreeName)
			if err != nil {
				return err
			}

			return printDir(stdout, path)
		},
	}

	cmd.Flags().SetInterspersed(false)
	return cmd
}

func newInitCommand(stdout io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init <shell>",
		Short: "Print shell integration for directory switching",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("%w: init requires <shell>", errUsage)
			}
			if !slices.Contains([]string{"bash", "zsh"}, args[0]) {
				return fmt.Errorf("%w: unsupported shell %q", errUsage, args[0])
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprint(stdout, shellInitScript(args[0]))
			return err
		},
	}

	cmd.Flags().SetInterspersed(false)
	return cmd
}

func shellInitScript(shell string) string {
	return fmt.Sprintf(`# gm shell integration for %s
gm() {
  local out exit_code line dir display
  out="$(%s=1 command gm "$@")"
  exit_code=$?
  if [ $exit_code -ne 0 ]; then
    return $exit_code
  fi

  while IFS= read -r line; do
    case "$line" in
      %s*)
        dir="${line#%s}"
        ;;
      *)
        if [ -n "$display" ]; then
          display="$display
$line"
        else
          display="$line"
        fi
        ;;
    esac
  done <<EOF
$out
EOF

  [ -n "$display" ] && printf '%%s\n' "$display"
  [ -n "$dir" ] && builtin cd -- "$dir"
}
`, shell, shellIntegrationEnv, shellCDPrefix, shellCDPrefix)
}

func newFeatCommand(manager **repo.Manager, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "feat",
		Short: "Manage feature worktrees",
	}
	cmd.AddCommand(newFeatAddCommand(manager, stdout))
	cmd.AddCommand(newFeatSyncCommand(manager, stdout))
	cmd.AddCommand(newFeatPruneCommand(manager, stdout, stderr))
	return cmd
}

func newFeatAddCommand(manager **repo.Manager, stdout io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <name> [base]",
		Short: "Create a feature worktree",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 || len(args) > 2 {
				return fmt.Errorf("%w: feat add requires <name> [base]", errUsage)
			}
			if len(args) == 2 && args[1] != repo.DefaultWorktreeName && args[1] != "upstream/"+repo.DefaultWorktreeName {
				return fmt.Errorf("%w: unsupported base %q; expected %q or %q", errUsage, args[1], repo.DefaultWorktreeName, "upstream/"+repo.DefaultWorktreeName)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			base := repo.DefaultWorktreeName
			if len(args) == 2 {
				base = args[1]
			}
			path, err := (*manager).AddFeatureWorktree(cmd.Context(), repo.FeatureAddOptions{
				Name: args[0],
				Base: base,
			})
			if err != nil {
				return err
			}
			return printDir(stdout, path)
		},
	}
	cmd.Flags().SetInterspersed(false)
	return cmd
}

func newFeatSyncCommand(manager **repo.Manager, stdout io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync [remote]",
		Short: "Sync the current feature branch to a remote",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return fmt.Errorf("%w: feat sync requires [remote]", errUsage)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			remote := ""
			if len(args) == 1 {
				remote = args[0]
			}
			if _, err := (*manager).SyncFeatureWorktree(cmd.Context(), repo.FeatureSyncOptions{
				StartDir: cwd,
				Remote:   remote,
			}); err != nil {
				return err
			}
			return printDir(stdout, cwd)
		},
	}
	cmd.Flags().SetInterspersed(false)
	return cmd
}

func newFeatPruneCommand(manager **repo.Manager, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove synced feature worktrees whose remote branches are gone",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return fmt.Errorf("%w: feat prune takes no arguments", errUsage)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := (*manager).PruneFeatureWorktrees(cmd.Context(), repo.FeaturePruneOptions{
				Stderr: stderr,
			})
			if err != nil {
				return err
			}
			for _, path := range result.RemovedPaths {
				if _, err := fmt.Fprintln(stdout, path); err != nil {
					return err
				}
			}
			if result.FinalDir != "" {
				return printDir(stdout, result.FinalDir)
			}
			return nil
		},
	}
	cmd.Flags().SetInterspersed(false)
	return cmd
}

func commandUnder(cmd *cobra.Command, name string) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() == name {
			return true
		}
	}
	return false
}

func usageError(cmd *cobra.Command, message string) error {
	_ = cmd.Usage()
	if message == "" {
		return errUsage
	}
	return fmt.Errorf("%w: %s", errUsage, message)
}

func printDir(stdout io.Writer, path string) error {
	if os.Getenv(shellIntegrationEnv) != "" {
		_, err := fmt.Fprintf(stdout, "%s%s\n", shellCDPrefix, path)
		return err
	}
	_, err := fmt.Fprintln(stdout, path)
	return err
}

func configureBaseFlag(flags *pflag.FlagSet, baseDir *string) {
	flags.StringVar(baseDir, "base", "", "base directory for managed repos")
}

func configureConfigFlag(flags *pflag.FlagSet, configPath *string) {
	flags.StringVar(configPath, "config", "", "path to gm YAML config file")
}
