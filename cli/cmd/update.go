package cmd

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wolzey/agent-factory/cli/internal/hooks"
	"github.com/wolzey/agent-factory/cli/internal/ui"
)

const (
	repo         = "wolzey/agent-factory"
	releasesAPI  = "https://api.github.com/repos/" + repo + "/releases/latest"
	downloadBase = "https://github.com/" + repo + "/releases/download"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update Agent Factory CLI to the latest version",
	RunE:  runUpdate,
}

var forceUpdate bool

// errUpdateDeclined gives a refusal a non-zero exit. A provisioning script that
// runs `update` on a source-built or unrankable binary would otherwise read the
// zero from a refusal as "you are current" and ship a stale binary — before this
// command learned to decline, that invocation always upgraded. Being already on
// the latest release is success rather than a refusal, and still exits zero —
// unless the repair below fails, which is the only work that path does.
var errUpdateDeclined = fmt.Errorf("update declined: %w", errReported)

func init() {
	updateCmd.Flags().BoolVarP(&forceUpdate, "force", "f", false,
		"Replace this binary with the latest release even if that is a downgrade, an unrankable tag, or a source build")
	// Every failure here is already reported through ui, so Cobra printing the
	// error itself would duplicate it. Execute() still prints what it gets back,
	// which is why the errors it has already shown wrap errReported. Usage is
	// silenced inside RunE rather than here: a mistyped flag fails before RunE
	// runs, and that is the one case where the flag list is what the reader needs.
	updateCmd.SilenceErrors = true
}

type ghRelease struct {
	TagName string `json:"tag_name"`
}

// updateAction is what to do with the tag GitHub reports as latest.
type updateAction int

const (
	updateProceed updateAction = iota // download and replace
	updateSkip                        // nothing to do; not an error
	updateBlock                       // replacing would lose or downgrade something
)

// planUpdate decides whether replacing this binary with latest is the right
// move, and returns the line to print about it. It reaches nothing, so the
// decision is testable without a release on GitHub.
func planUpdate(current, latest string, force bool) (updateAction, string) {
	if force {
		return updateProceed, ""
	}

	// A source build's contents are unknown to us: it may carry local changes, or
	// fixes that no release has yet. Overwriting it is a decision, not a default.
	if current == devVersion {
		return updateBlock, fmt.Sprintf(
			"This binary was built from source, so it has no release tag to compare against %s.\n"+
				"  Re-run with --force to replace it anyway.", latest)
	}

	// Proceeding on an unrankable pair would reopen the hole this command exists to
	// close: "not provably newer" is not "newer". Since comparePrerelease ranks
	// pre-releases, what reaches here is a tag that is not semver at all — an empty
	// or malformed `tag_name`, or a scheme this does not parse. Refusing beats
	// guessing, and --force is one flag away.
	cmp, ok := compareVersions(current, latest)
	if !ok {
		return updateBlock, fmt.Sprintf(
			"Cannot tell whether %s is newer or older than the installed %s, so this may be a downgrade.\n"+
				"  Re-run with --force to install %s anyway.", latest, current, latest)
	}
	switch {
	case cmp == 0:
		return updateSkip, fmt.Sprintf("Already on %s — nothing to do.", latest)
	case cmp > 0:
		return updateBlock, fmt.Sprintf(
			"Installed %s is newer than the latest release %s; refusing to downgrade.\n"+
				"  Re-run with --force if that is what you want.", current, latest)
	default:
		return updateProceed, ""
	}
}

func runUpdate(cmd *cobra.Command, args []string) error {
	// Flags parsed, so anything that fails from here is a runtime problem already
	// reported through ui, not something the usage text would help with.
	cmd.SilenceUsage = true

	ui.PrintBanner()

	// Fetch latest release tag
	ui.Info("Checking for updates...")
	fmt.Println()

	resp, err := http.Get(releasesAPI)
	if err != nil {
		ui.Error("Failed to check for updates: " + err.Error())
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		ui.Error(fmt.Sprintf("GitHub API returned %d", resp.StatusCode))
		return fmt.Errorf("github API error: %d", resp.StatusCode)
	}

	var release ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		ui.Error("Failed to parse release info: " + err.Error())
		return err
	}

	fmt.Printf("  Installed:      %s\n", ui.CyanStyle.Render(version))
	fmt.Printf("  Latest version: %s\n", ui.CyanStyle.Render(release.TagName))
	fmt.Println()

	if action, message := planUpdate(version, release.TagName, forceUpdate); action != updateProceed {
		// Being on the latest release is success and stays on stdout at exit 0; a
		// refusal is a diagnostic that comes with a non-zero exit, so it goes to
		// stderr where a CI step that only surfaces stderr on failure will see it.
		if action == updateSkip {
			ui.Success(message)
		} else {
			ui.WarnErr(message)
		}

		// refreshInstalledAssets writes the *running* binary's embedded copies, so
		// it repairs only when the binary running is the installed one. A source
		// build almost never is: a contributor checking for a release from a feature
		// branch would otherwise overwrite the installed registration, skills and
		// identity with in-progress ones, while being told the binary itself was
		// left alone. `install` is the command that adopts a build deliberately.
		//
		// Those three are all this guard protects. The hook *script* is synced by
		// the root's PersistentPreRun before any command body runs, from any binary
		// — that is the privacy repair path and predates this guard, so neither the
		// message below nor this comment may claim the script was left alone.
		if version == devVersion {
			// Only worth saying to someone who has something installed to protect.
			if len(hooks.InstalledTargets()) > 0 {
				ui.WarnErr("Hook registration and skill files were left as they are; " +
					"run 'agent-factory install' to adopt this build.")
			}
			fmt.Println()
			return errUpdateDeclined
		}

		// Otherwise the assets should match the binary that is running. Downloading
		// an identical release was how a clobbered settings.json entry or a deleted
		// skill file got repaired, because the fresh binary ran _refresh-assets
		// afterwards; the download is the part worth skipping, not the repair. The
		// root's hook is no substitute — it only rewrites the script, and never
		// touches registration, skills or identity.
		installed := len(hooks.InstalledTargets()) > 0
		if err := refreshInstalledAssets(); err != nil {
			// On the skip path this repair is the only work the command does, and
			// the README sells `update` for exactly it. Exiting zero here would
			// hand a provisioning script a success it did not get.
			ui.WarnErr("Installed hooks could not be refreshed: " + err.Error() +
				"\n  Run 'agent-factory install' to refresh them manually.")
			fmt.Println()
			return fmt.Errorf("refresh installed assets: %w", errReported)
		}
		if installed {
			// Nothing is installed for someone who has never run `install`, and
			// claiming a refresh there would be a plain untruth.
			// Not "and identity": that half only happens when a config exists.
			ui.Success("Installed hooks refreshed")
		}
		fmt.Println()
		if action == updateBlock {
			return errUpdateDeclined
		}
		return nil
	}

	// Determine platform asset name
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	asset := fmt.Sprintf("agent-factory_%s_%s.tar.gz", goos, goarch)
	downloadURL := fmt.Sprintf("%s/%s/%s", downloadBase, release.TagName, asset)

	fmt.Printf("  Platform:       %s/%s\n", goos, goarch)
	fmt.Println()

	// Download
	ui.Info("Downloading " + asset + "...")

	dlResp, err := http.Get(downloadURL)
	if err != nil {
		ui.Error("Download failed: " + err.Error())
		return err
	}
	defer dlResp.Body.Close()

	if dlResp.StatusCode != 200 {
		ui.Error(fmt.Sprintf("Download returned %d — is %s available for your platform?", dlResp.StatusCode, release.TagName))
		return fmt.Errorf("download error: %d", dlResp.StatusCode)
	}

	// Extract the binary from the tarball
	binary, err := extractBinaryFromTarGz(dlResp.Body, "agent-factory")
	if err != nil {
		ui.Error("Failed to extract binary: " + err.Error())
		return err
	}

	// Find current binary path
	execPath, err := os.Executable()
	if err != nil {
		ui.Error("Cannot determine current binary path: " + err.Error())
		return err
	}

	// Resolve symlinks
	resolvedPath, err := resolveSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}

	// Replace the directory entry, never overwrite the running executable.
	ui.Info("Installing to " + resolvedPath + "...")

	if err := replaceBinary(resolvedPath, binary); err != nil {
		ui.Error("Failed to write binary: " + err.Error())
		ui.Info("You may need to run with sudo or check file permissions.")
		return err
	}

	fmt.Println()
	ui.Success(fmt.Sprintf("Updated to %s!", release.TagName))

	// Run the newly installed binary so refreshed hooks and skills come from the
	// new release rather than this still-running executable's embedded assets.
	refresh := exec.Command(resolvedPath, "_refresh-assets")
	if output, err := refresh.CombinedOutput(); err != nil {
		ui.Warn("CLI updated, but installed hooks could not be refreshed: " + err.Error())
		if message := strings.TrimSpace(string(output)); message != "" {
			ui.Info(message)
		}
		ui.Info("Run 'agent-factory install' to refresh hooks manually.")
	} else {
		ui.Success("Installed hooks and identity refreshed")
	}

	fmt.Println()
	return nil
}

func extractBinaryFromTarGz(r io.Reader, name string) ([]byte, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("gzip error: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tar error: %w", err)
		}

		// Match the binary name (may be in a subdirectory)
		if header.Typeflag == tar.TypeReg && strings.HasSuffix(header.Name, name) {
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("read error: %w", err)
			}
			return data, nil
		}
	}

	return nil, fmt.Errorf("binary %q not found in archive", name)
}

// replaceBinary stages a fresh inode beside the executable, then atomically
// renames it into place. In-place writes can leave macOS's cached code signature
// stale (SIGKILL on launch) and fail with ETXTBSY on Linux for a running binary.
func replaceBinary(path string, binary []byte) error {
	staged, err := os.CreateTemp(filepath.Dir(path), ".agent-factory-update-*")
	if err != nil {
		return fmt.Errorf("stage executable: %w", err)
	}
	defer os.Remove(staged.Name())
	defer staged.Close()

	if _, err := staged.Write(binary); err != nil {
		return fmt.Errorf("write staged executable: %w", err)
	}
	if err := staged.Chmod(0o755); err != nil {
		return fmt.Errorf("set executable permissions: %w", err)
	}
	if err := staged.Sync(); err != nil {
		return fmt.Errorf("sync staged executable: %w", err)
	}
	if err := staged.Close(); err != nil {
		return fmt.Errorf("close staged executable: %w", err)
	}
	if err := os.Rename(staged.Name(), path); err != nil {
		return fmt.Errorf("replace executable: %w", err)
	}
	return nil
}

func resolveSymlinks(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}
