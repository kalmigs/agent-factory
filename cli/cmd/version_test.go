package cmd

import "testing"

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
