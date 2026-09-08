package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wolzey/agent-factory/cli/internal/hooks"
)

var rootCmd = &cobra.Command{
	Use:     "agent-factory",
	Short:   "Agent Factory CLI - install/uninstall Claude/Codex visualization hooks",
	Long:    "Install and manage Agent Factory hooks for Claude Code and Codex.\nYour coding sessions will appear as pixel art avatars in a retro arcade.",
	Version: version,
	// `update` rewrites the binary from inside the old process, so the old code
	// finishes that run and a changed hook script is never written by the upgrade
	// delivering it. Repair it here instead, on the first run of the new binary,
	// so a fix to what the hook sends cannot sit undeployed on someone's machine.
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		updated, err := hooks.SyncHookScript()
		switch {
		case err != nil:
			// Failing quietly here would leave an older script in place, still
			// forwarding raw payloads, with nothing to indicate it.
			fmt.Fprintln(os.Stderr, "agent-factory: could not update the hook script: "+err.Error())
			fmt.Fprintln(os.Stderr, "agent-factory: run 'agent-factory install' to reinstall it")
		case updated:
			fmt.Fprintln(os.Stderr, "agent-factory: hook script updated to match this version")
		}
	},
}

// errReported marks an error whose explanation has already reached the user, so
// Execute() below contributes the exit status and nothing else. It lives here
// because Execute() is what honours it; commands only opt in by wrapping it.
var errReported = errors.New("already reported")

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		// Some errors have already explained themselves in full; printing them here
		// would add a bare, context-free line under that. The exit status is the
		// only part of those still worth delivering.
		if !errors.Is(err, errReported) {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}

func init() {
	// Keep `--version` and `version` printing the same string; a release that
	// reports itself two ways is the confusion this is meant to end.
	rootCmd.SetVersionTemplate("agent-factory {{.Version}}\n")

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(avatarCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(emoteCmd)
	rootCmd.AddCommand(chatCmd)
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(connectCmd)
	rootCmd.AddCommand(tokenCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(refreshAssetsCmd)
}
