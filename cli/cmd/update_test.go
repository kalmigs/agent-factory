package cmd

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceBinaryUsesNewInode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent-factory")
	if err := os.WriteFile(path, []byte("old executable"), 0o755); err != nil {
		t.Fatal(err)
	}
	old, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	before, err := old.Stat()
	if err != nil {
		t.Fatal(err)
	}

	if err := replaceBinary(path, []byte("new executable")); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Fatal("updated executable reuses the old inode")
	}
	if after.Mode().Perm() != 0o755 {
		t.Fatalf("permissions = %o, want 755", after.Mode().Perm())
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "new executable" {
		t.Fatalf("replacement contents = %q, error = %v", contents, err)
	}
	contents, err = io.ReadAll(old)
	if err != nil || string(contents) != "old executable" {
		t.Fatalf("old executable was modified: contents = %q, error = %v", contents, err)
	}
	assertNoStagedBinary(t, dir)
}

func TestReplaceBinaryCleansUpWhenRenameFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent-factory")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := replaceBinary(path, []byte("new executable")); err == nil {
		t.Fatal("expected replacement of a directory to fail")
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		t.Fatalf("original destination changed: %v", err)
	}
	assertNoStagedBinary(t, dir)
}

func TestReplaceBinaryPreservesSymlinkChain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent-factory")
	if err := os.WriteFile(path, []byte("old executable"), 0o755); err != nil {
		t.Fatal(err)
	}
	for link, target := range map[string]string{"first": "second", "second": "agent-factory"} {
		if err := os.Symlink(target, filepath.Join(dir, link)); err != nil {
			t.Fatal(err)
		}
	}
	resolved, err := resolveSymlinks(filepath.Join(dir, "first"))
	if err != nil {
		t.Fatal(err)
	}
	if err := replaceBinary(resolved, []byte("new executable")); err != nil {
		t.Fatal(err)
	}
	for link, target := range map[string]string{"first": "second", "second": "agent-factory"} {
		got, err := os.Readlink(filepath.Join(dir, link))
		if err != nil || got != target {
			t.Fatalf("symlink %s changed: target = %q, error = %v", link, got, err)
		}
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "new executable" {
		t.Fatalf("target contents = %q, error = %v", contents, err)
	}
}

func TestPlanUpdate(t *testing.T) {
	cases := []struct {
		name            string
		current, latest string
		force           bool
		want            updateAction
		wantMessage     bool
	}{
		{
			name:    "a newer release is installed",
			current: "v0.21.1", latest: "v0.22.1",
			want: updateProceed,
		},
		{
			name:    "already on the latest release",
			current: "v0.22.1", latest: "v0.22.1",
			want: updateSkip, wantMessage: true,
		},
		{
			// The window this command was built for: between 2026-04 and 2026-09
			// the latest release was five months behind main, and running update
			// on a current binary would have walked it backwards.
			name:    "the latest release is older than what is installed",
			current: "v0.22.1", latest: "v0.21.1",
			want: updateBlock, wantMessage: true,
		},
		{
			name:    "a downgrade is allowed once asked for",
			current: "v0.22.1", latest: "v0.21.1", force: true,
			want: updateProceed,
		},
		{
			// A source build may carry local changes, or fixes no release has.
			name:    "a source build is not replaced by default",
			current: devVersion, latest: "v0.22.1",
			want: updateBlock, wantMessage: true,
		},
		{
			name:    "a source build is replaced once asked for",
			current: devVersion, latest: "v0.22.1", force: true,
			want: updateProceed,
		},
		{
			// "Not provably newer" is not "newer". Two pre-releases of one version
			// are the reachable case, and rc2 → rc1 is the downgrade to avoid.
			name:    "tags that cannot be ranked are not assumed to be an upgrade",
			current: "v1.0.0-rc2", latest: "v1.0.0-rc1",
			want: updateBlock, wantMessage: true,
		},
		{
			name:    "an unrankable pair still updates once asked for",
			current: "v1.0.0-rc2", latest: "v1.0.0-rc1", force: true,
			want: updateProceed,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			action, message := planUpdate(c.current, c.latest, c.force)
			if action != c.want {
				t.Errorf("action = %d, want %d", action, c.want)
			}
			if (message != "") != c.wantMessage {
				t.Errorf("message = %q, want a message: %v", message, c.wantMessage)
			}
		})
	}
}

func assertNoStagedBinary(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".agent-factory-update-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("staged files remain: %v, error = %v", matches, err)
	}
}
