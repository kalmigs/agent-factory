package cmd

import (
	"os"
	"strings"
	"testing"
)

func TestCompareVersionsRanksReleases(t *testing.T) {
	cases := []struct {
		a, b string
		want int // sign of the expected result
	}{
		{"v0.22.1", "v0.22.1", 0},
		{"0.22.1", "v0.22.1", 0}, // the leading v is decoration
		{"v0.21.1", "v0.22.0", -1},
		{"v0.22.0", "v0.21.1", 1},
		{"v0.22.1", "v0.22.10", -1}, // not a string comparison
		{"v1.0.0", "v0.99.99", 1},
		{"v1.2", "v1.2.0", 0}, // a missing field is zero, not shorter
		{"v1.2", "v1.2.1", -1},
		{"v1.0.0-rc1", "v1.0.0", -1}, // a pre-release precedes its release
		{"v1.0.0", "v1.0.0-rc1", 1},
		{"v1.0.0-rc1", "v1.0.0-rc1", 0},
		{"v1.0.0+build.5", "v1.0.0", 0}, // build metadata does not rank
	}

	for _, c := range cases {
		got, ok := compareVersions(c.a, c.b)
		if !ok {
			t.Errorf("compareVersions(%q, %q) could not rank them", c.a, c.b)
			continue
		}
		if sign(got) != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want sign %d", c.a, c.b, got, c.want)
		}
	}
}

func TestCompareVersionsDeclinesWhatItCannotRank(t *testing.T) {
	// Each of these must report "unknown" rather than guess. A wrong "equal"
	// here would skip a real update; a wrong "newer" would block one.
	cases := [][2]string{
		{devVersion, "v0.22.1"},
		{"v0.22.1", devVersion},
		{"", "v0.22.1"},
		{"v1.0.0-rc1", "v1.0.0-rc2"}, // same release, unrankable pre-releases
		{"v1.2.x", "v1.2.3"},
		{"nightly", "v1.0.0"},
	}

	for _, c := range cases {
		if got, ok := compareVersions(c[0], c[1]); ok {
			t.Errorf("compareVersions(%q, %q) = %d, ok — expected it to decline", c[0], c[1], got)
		}
	}
}

func TestVersionDefaultsToDevInASourceBuild(t *testing.T) {
	// go test never passes the -X flag, so this is the un-injected value. If it
	// ever reads as a release tag, a source build is claiming to be one.
	if version != devVersion {
		t.Fatalf("version = %q in a source build, want %q", version, devVersion)
	}
	if _, _, ok := splitVersion(devVersion); ok {
		t.Fatal("the dev sentinel parses as a release tag, so it can be ranked against one")
	}
}

// TestReleaseInjectsThisPackage guards the one failure this whole file exists to
// prevent, and the one nothing else would catch: `go build` silently ignores an
// -X whose import path does not resolve. Rename the module, move this package or
// mistype the path and releases go back to reporting "dev" — with a green build,
// a green test run, and no sign of it until someone downloads a binary.
func TestReleaseInjectsThisPackage(t *testing.T) {
	module := modulePath(t)
	config, err := os.ReadFile("../.goreleaser.yaml")
	if err != nil {
		t.Fatalf("read goreleaser config: %v", err)
	}

	// Look only at live list items inside an ldflags block: a commented-out entry
	// still contains the string but injects nothing, and that is precisely the
	// green-build-broken-release this test is here to catch.
	var ldflag string
	var blocks, builds int
	inLdflags := false
	for _, line := range strings.Split(string(config), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- main:") {
			builds++
		}
		if trimmed == "ldflags:" {
			blocks++
			inLdflags = true
			continue
		}
		if !inLdflags || trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(trimmed, "- ") {
			inLdflags = false // the list ended; anything below is another key
			continue
		}
		if ldflag == "" && strings.Contains(trimmed, "cmd.version=") {
			ldflag = trimmed
		}
	}

	if ldflag == "" {
		t.Fatal("no live ldflags entry injects cmd.version; released binaries would report \"dev\"")
	}
	if !strings.HasPrefix(ldflag, "- -X ") {
		t.Errorf("ldflag entry is %q, want a plain `- -X …` list item", ldflag)
	}
	// One build, one ldflags block: a second of either could ship an artifact with
	// no version injected, which this line-scan would not otherwise notice.
	if builds != 1 || blocks != 1 {
		t.Errorf("found %d build(s) and %d ldflags block(s), want 1 of each — "+
			"give every build the cmd.version ldflag and widen this test", builds, blocks)
	}
	if want := "-X " + module + "/cmd.version="; !strings.Contains(ldflag, want) {
		t.Errorf("ldflag is %q, want it to target %q", ldflag, want)
	}
	// .Version drops the leading v, which would no longer match the tag_name
	// `update` compares against.
	if !strings.Contains(ldflag, "{{ .Tag }}") {
		t.Errorf("ldflag is %q, want the injected value to be {{ .Tag }}", ldflag)
	}
}

func modulePath(t *testing.T) string {
	t.Helper()
	gomod, err := os.ReadFile("../go.mod")
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	for _, line := range strings.Split(string(gomod), "\n") {
		if path, found := strings.CutPrefix(line, "module "); found {
			return strings.TrimSpace(path)
		}
	}
	t.Fatal("go.mod declares no module path")
	return ""
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}
