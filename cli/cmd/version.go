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
// ok is false when either tag has a field that is not a number — "dev" and any
// other free-form string. Callers should treat that as "unknown", not as "equal":
// the whole point of this file is that the CLI stops guessing which build it is.
// Note there is no shortcut for a == b, so two identical unparseable strings are
// still reported as unrankable rather than as the same release.
func compareVersions(a, b string) (int, bool) {
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

	return comparePrerelease(aPre, bPre), true
}

// comparePrerelease orders the suffix left over after the numbers, by semver's
// rules: a release outranks any pre-release of the same version, identifiers are
// compared field by field — numeric ones numerically, the rest by ASCII, and a
// numeric identifier below an alphanumeric one — and when every shared field
// matches, the tag with more of them wins.
//
// Every pair has an answer here, which is the point: rc1 → rc2 is an ordinary
// upgrade, and refusing it because the suffix "cannot be ranked" would make the
// downgrade guard cost more than it saves.
//
// The ASCII rule has a sharp edge semver keeps deliberately: "rc10" sorts below
// "rc2", because they are single identifiers rather than "rc" and a number. So
// rc9 → rc10 reads as a downgrade and is refused. Matching the spec is worth more
// than smoothing that over with an ordering nothing else in the world uses, and
// --force covers anyone who hits it. Tag pre-releases as -rc.10 and the number
// becomes its own field, compared numerically.
func comparePrerelease(a, b string) int {
	switch {
	case a == b:
		return 0
	case a == "":
		return 1 // a release outranks a pre-release of the same version
	case b == "":
		return -1
	}

	aFields, bFields := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(aFields) && i < len(bFields); i++ {
		if aFields[i] == bFields[i] {
			continue
		}
		an, aErr := strconv.Atoi(aFields[i])
		bn, bErr := strconv.Atoi(bFields[i])
		switch {
		case aErr == nil && bErr == nil:
			if an != bn {
				return an - bn
			}
		case aErr == nil:
			return -1
		case bErr == nil:
			return 1
		default:
			return strings.Compare(aFields[i], bFields[i])
		}
	}
	return len(aFields) - len(bFields)
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
