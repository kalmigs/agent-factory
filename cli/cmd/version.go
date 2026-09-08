package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// devVersion is what a binary reports when nothing injected a release tag —
// a `go build` from a source checkout. It is deliberately not a number, so it
// can never be mistaken for a release and never compares as one.
const devVersion = "dev"

// version is the release tag this binary was built from. GoReleaser overwrites
// it through -ldflags "-X …/cli/cmd.version={{ .Tag }}"; see .goreleaser.yaml,
// which explains why .Tag and not .Version. TestReleaseInjectsThisPackage keeps
// the two in step.
var version = devVersion

// versionCmd exists because `agent-factory version` is what people type, while
// Cobra only wires --version onto the root command.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version this binary was built from",
	// Asking a binary what it is should not change anything on disk. Cobra runs
	// only the first persistent hook it finds walking up from the command, so
	// this empty override suppresses the root's hook-script sync. The --version
	// flag needs no equivalent: Cobra returns on it before any hook runs.
	PersistentPreRun: func(cmd *cobra.Command, args []string) {},
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "agent-factory "+version)
		return err
	},
}

// compareVersions orders two release tags, returning a negative number when a
// precedes b, zero when they are the same release, and a positive number when a
// follows b. Tags may carry a leading "v" and a pre-release suffix.
//
// ok is false when the two cannot be ranked honestly: a non-numeric field, or
// equal numbers with differing pre-release suffixes ("v1.0.0-rc1" against
// "v1.0.0-rc2"). Callers should treat that as "unknown", not as "equal" — the
// whole point of this file is that the CLI stops guessing which build it is.
func compareVersions(a, b string) (int, bool) {
	if a == b {
		return 0, true
	}

	aNums, aPre, aOK := splitVersion(a)
	bNums, bPre, bOK := splitVersion(b)
	if !aOK || !bOK {
		return 0, false
	}

	for i := 0; i < len(aNums) || i < len(bNums); i++ {
		// A tag with fewer fields is not shorter, it is zero there: v1.2 is v1.2.0.
		an, bn := 0, 0
		if i < len(aNums) {
			an = aNums[i]
		}
		if i < len(bNums) {
			bn = bNums[i]
		}
		if an != bn {
			return an - bn, true
		}
	}

	// Same numbers. A pre-release precedes the release it leads to; two different
	// pre-releases of the same version are not worth ranking here.
	switch {
	case aPre == bPre:
		return 0, true
	case aPre == "":
		return 1, true
	case bPre == "":
		return -1, true
	default:
		return 0, false
	}
}

// splitVersion separates "v1.2.3-rc1" into its numeric fields and the
// pre-release suffix. ok is false if any field before the suffix is not a
// number, which is how "dev" and any other free-form string get rejected.
func splitVersion(tag string) (nums []int, pre string, ok bool) {
	tag = strings.TrimPrefix(strings.TrimSpace(tag), "v")
	if tag == "" {
		return nil, "", false
	}
	// Build metadata ("+abc") never affects ordering, so drop it before anything.
	if i := strings.IndexByte(tag, '+'); i >= 0 {
		tag = tag[:i]
	}
	if i := strings.IndexByte(tag, '-'); i >= 0 {
		tag, pre = tag[:i], tag[i+1:]
	}

	for _, field := range strings.Split(tag, ".") {
		n, err := strconv.Atoi(field)
		if err != nil || n < 0 {
			return nil, "", false
		}
		nums = append(nums, n)
	}
	return nums, pre, true
}
