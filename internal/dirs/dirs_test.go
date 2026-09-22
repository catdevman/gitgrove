package dirs

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sourceTree builds a small directory tree to copy or link, and returns its path.
func sourceTree(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "src")
	mustMkdir(t, filepath.Join(root, "sub"))
	mustWrite(t, filepath.Join(root, "README.md"), "# doc\n", 0o644)
	mustWrite(t, filepath.Join(root, "sub", "nested.txt"), "nested\n", 0o600)
	mustWrite(t, filepath.Join(root, "run.sh"), "#!/bin/sh\n", 0o755)
	if err := os.Symlink("../README.md", filepath.Join(root, "sub", "up")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	return root
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWrite(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func TestValidMode(t *testing.T) {
	for _, tc := range []struct {
		mode string
		want bool
	}{
		{ModeCopy, true},
		{ModeSymlink, true},
		{"", false},
		{"move", false},
		{"COPY", false},
	} {
		if got := ValidMode(tc.mode); got != tc.want {
			t.Errorf("ValidMode(%q) = %v, want %v", tc.mode, got, tc.want)
		}
	}
}

func TestInspectMissing(t *testing.T) {
	st, err := Inspect(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if st.Present {
		t.Errorf("Present = true for a path that does not exist")
	}
}

func TestInspectDirectory(t *testing.T) {
	dir := t.TempDir()
	st, err := Inspect(dir)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if !st.Present || st.IsSymlink {
		t.Errorf("Inspect(dir) = %+v, want present and not a symlink", st)
	}
}

func TestInspectSymlinkAbsoluteTarget(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "target")
	mustMkdir(t, target)
	link := filepath.Join(tmp, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	st, err := Inspect(link)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if !st.Present || !st.IsSymlink {
		t.Fatalf("Inspect = %+v, want a present symlink", st)
	}
	if st.Target != target {
		t.Errorf("Target = %q, want %q", st.Target, target)
	}
}

// A relative symlink target has to be resolved against the link's own
// directory, or a drift check would compare a relative path to an absolute one
// and retarget a link that is in fact correct.
func TestInspectSymlinkRelativeTargetIsResolved(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "target")
	mustMkdir(t, target)
	link := filepath.Join(tmp, "link")
	if err := os.Symlink("target", link); err != nil {
		t.Fatal(err)
	}

	st, err := Inspect(link)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if st.Target != target {
		t.Errorf("Target = %q, want the resolved %q", st.Target, target)
	}
}

func TestEnsureCopyCreatesTree(t *testing.T) {
	src := sourceTree(t)
	dest := filepath.Join(t.TempDir(), "docs")

	action, err := Ensure(src, dest, ModeCopy)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if !strings.HasPrefix(action, "copied ") {
		t.Errorf("action = %q, want a copied... message", action)
	}
	if got := readFile(t, filepath.Join(dest, "README.md")); got != "# doc\n" {
		t.Errorf("copied README.md = %q", got)
	}
	if got := readFile(t, filepath.Join(dest, "sub", "nested.txt")); got != "nested\n" {
		t.Errorf("copied nested.txt = %q", got)
	}
}

// Re-running sync must not throw away work done inside the grove copy.
func TestEnsureCopyIsIdempotentAndKeepsEdits(t *testing.T) {
	src := sourceTree(t)
	dest := filepath.Join(t.TempDir(), "docs")

	if _, err := Ensure(src, dest, ModeCopy); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dest, "README.md"), "edited in the grove\n", 0o644)

	action, err := Ensure(src, dest, ModeCopy)
	if err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if action != "" {
		t.Errorf("action = %q, want no action on an existing copy", action)
	}
	if got := readFile(t, filepath.Join(dest, "README.md")); got != "edited in the grove\n" {
		t.Errorf("README.md = %q, want the edit preserved", got)
	}
}

// Switching an entry from symlink mode to copy mode replaces the link. The
// source must survive: unlinking never touches what a link points at.
func TestEnsureCopyReplacesLeftoverSymlink(t *testing.T) {
	src := sourceTree(t)
	dest := filepath.Join(t.TempDir(), "docs")

	if _, err := Ensure(src, dest, ModeSymlink); err != nil {
		t.Fatal(err)
	}
	if _, err := Ensure(src, dest, ModeCopy); err != nil {
		t.Fatalf("Ensure copy over symlink: %v", err)
	}

	st, err := Inspect(dest)
	if err != nil {
		t.Fatal(err)
	}
	if st.IsSymlink {
		t.Errorf("destination is still a symlink after switching to copy mode")
	}
	if _, err := os.Stat(filepath.Join(src, "README.md")); err != nil {
		t.Errorf("source was damaged: %v", err)
	}
}

func TestEnsureSymlinkCreates(t *testing.T) {
	src := sourceTree(t)
	dest := filepath.Join(t.TempDir(), "docs")

	action, err := Ensure(src, dest, ModeSymlink)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if !strings.HasPrefix(action, "symlinked ") {
		t.Errorf("action = %q, want a symlinked... message", action)
	}
	st, _ := Inspect(dest)
	if !st.IsSymlink || st.Target != src {
		t.Errorf("Inspect = %+v, want a symlink to %s", st, src)
	}
}

func TestEnsureSymlinkIsIdempotent(t *testing.T) {
	src := sourceTree(t)
	dest := filepath.Join(t.TempDir(), "docs")

	if _, err := Ensure(src, dest, ModeSymlink); err != nil {
		t.Fatal(err)
	}
	action, err := Ensure(src, dest, ModeSymlink)
	if err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if action != "" {
		t.Errorf("action = %q, want no action for an already-correct symlink", action)
	}
}

func TestEnsureSymlinkRetargetsWhenSourceChanged(t *testing.T) {
	tmp := t.TempDir()
	oldSrc := filepath.Join(tmp, "old")
	newSrc := filepath.Join(tmp, "new")
	mustMkdir(t, oldSrc)
	mustMkdir(t, newSrc)
	dest := filepath.Join(tmp, "grove-entry")

	if _, err := Ensure(oldSrc, dest, ModeSymlink); err != nil {
		t.Fatal(err)
	}
	action, err := Ensure(newSrc, dest, ModeSymlink)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if !strings.HasPrefix(action, "retargeted ") {
		t.Errorf("action = %q, want a retargeted... message", action)
	}
	st, _ := Inspect(dest)
	if st.Target != newSrc {
		t.Errorf("Target = %q, want %q", st.Target, newSrc)
	}
	if _, err := os.Stat(oldSrc); err != nil {
		t.Errorf("old source was removed: %v", err)
	}
}

// A real directory in the way is not something gitgrove created, so sync must
// refuse rather than delete it.
func TestEnsureSymlinkRefusesRealDirectory(t *testing.T) {
	src := sourceTree(t)
	dest := filepath.Join(t.TempDir(), "docs")
	mustMkdir(t, dest)
	mustWrite(t, filepath.Join(dest, "mine.txt"), "important\n", 0o644)

	if _, err := Ensure(src, dest, ModeSymlink); err == nil {
		t.Fatal("Ensure succeeded, want an error for a real directory in the way")
	}
	if _, err := os.Stat(filepath.Join(dest, "mine.txt")); err != nil {
		t.Errorf("existing directory was damaged: %v", err)
	}
}

func TestEnsureUnknownMode(t *testing.T) {
	src := sourceTree(t)
	dest := filepath.Join(t.TempDir(), "docs")
	if _, err := Ensure(src, dest, "move"); err == nil {
		t.Fatal("Ensure succeeded, want an error for an unknown mode")
	}
}

func TestRemoveSymlinkLeavesSourceAlone(t *testing.T) {
	src := sourceTree(t)
	dest := filepath.Join(t.TempDir(), "docs")
	if _, err := Ensure(src, dest, ModeSymlink); err != nil {
		t.Fatal(err)
	}

	if err := Remove(dest, ModeSymlink, false); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Lstat(dest); !os.IsNotExist(err) {
		t.Errorf("link still present: %v", err)
	}
	if _, err := os.Stat(filepath.Join(src, "README.md")); err != nil {
		t.Errorf("source was removed along with the link: %v", err)
	}
}

// A copy is grove-owned scratch: prune deletes it without needing --force.
func TestRemoveCopyDeletesWithoutForce(t *testing.T) {
	src := sourceTree(t)
	dest := filepath.Join(t.TempDir(), "docs")
	if _, err := Ensure(src, dest, ModeCopy); err != nil {
		t.Fatal(err)
	}

	if err := Remove(dest, ModeCopy, false); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Errorf("copy still present: %v", err)
	}
	if _, err := os.Stat(filepath.Join(src, "README.md")); err != nil {
		t.Errorf("source was removed along with the copy: %v", err)
	}
}

func TestRemoveRealDirectoryInSymlinkModeNeedsForce(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "docs")
	mustMkdir(t, dest)
	mustWrite(t, filepath.Join(dest, "mine.txt"), "important\n", 0o644)

	if err := Remove(dest, ModeSymlink, false); err == nil {
		t.Fatal("Remove succeeded, want an error without force")
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("directory was deleted anyway: %v", err)
	}

	if err := Remove(dest, ModeSymlink, true); err != nil {
		t.Fatalf("forced Remove: %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Errorf("directory still present after forced Remove")
	}
}

func TestRemoveMissingIsNoOp(t *testing.T) {
	if err := Remove(filepath.Join(t.TempDir(), "gone"), ModeCopy, false); err != nil {
		t.Errorf("Remove on a missing path = %v, want nil", err)
	}
}

// Symlinks inside the tree are recreated, not followed: a directory full of
// links must not expand into a much larger copy.
func TestCopyDirRecreatesNestedSymlinks(t *testing.T) {
	src := sourceTree(t)
	dst := filepath.Join(t.TempDir(), "copy")

	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}

	linked := filepath.Join(dst, "sub", "up")
	fi, err := os.Lstat(linked)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("nested symlink was followed instead of recreated")
	}
	target, err := os.Readlink(linked)
	if err != nil {
		t.Fatal(err)
	}
	if target != "../README.md" {
		t.Errorf("symlink target = %q, want the original relative target", target)
	}
}

func TestCopyDirPreservesFileModes(t *testing.T) {
	src := sourceTree(t)
	dst := filepath.Join(t.TempDir(), "copy")
	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}

	for _, tc := range []struct {
		name string
		want os.FileMode
	}{
		{"run.sh", 0o755},
		{filepath.Join("sub", "nested.txt"), 0o600},
	} {
		fi, err := os.Stat(filepath.Join(dst, tc.name))
		if err != nil {
			t.Fatalf("stat %s: %v", tc.name, err)
		}
		if got := fi.Mode().Perm(); got != tc.want {
			t.Errorf("%s mode = %o, want %o", tc.name, got, tc.want)
		}
	}
}

func TestCopyDirRefusesToCopyIntoItself(t *testing.T) {
	src := sourceTree(t)
	for _, dst := range []string{src, filepath.Join(src, "inner")} {
		if err := CopyDir(src, dst); err == nil {
			t.Errorf("CopyDir(%s, %s) succeeded, want a refusal", src, dst)
		}
	}
}

func TestCopyDirReportsMissingSource(t *testing.T) {
	err := CopyDir(filepath.Join(t.TempDir(), "nope"), filepath.Join(t.TempDir(), "dst"))
	if err == nil {
		t.Fatal("CopyDir succeeded for a missing source")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("err = %v, want a not-exist error", err)
	}
}

// Sockets, devices and fifos have nothing meaningful to copy; they are skipped
// rather than failing the whole copy.
func TestCopyDirSkipsIrregularFiles(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	mustMkdir(t, src)
	mustWrite(t, filepath.Join(src, "keep.txt"), "kept\n", 0o644)

	ln, err := net.Listen("unix", filepath.Join(src, "sock"))
	if err != nil {
		t.Skipf("cannot create a unix socket here: %v", err)
	}
	defer ln.Close()

	dst := filepath.Join(t.TempDir(), "dst")
	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}
	if got := readFile(t, filepath.Join(dst, "keep.txt")); got != "kept\n" {
		t.Errorf("keep.txt = %q", got)
	}
	if _, err := os.Lstat(filepath.Join(dst, "sock")); !os.IsNotExist(err) {
		t.Errorf("socket was copied: %v", err)
	}
}

// --- error paths -------------------------------------------------------

func TestEnsureReportsAnUnreadableDestination(t *testing.T) {
	src := sourceTree(t)
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(parent, 0o700) })

	if _, err := Ensure(src, filepath.Join(parent, "docs"), ModeCopy); err == nil {
		t.Fatal("Ensure succeeded with an unreadable destination parent")
	}
}

func TestEnsureSymlinkReportsAMissingParent(t *testing.T) {
	src := sourceTree(t)
	dest := filepath.Join(t.TempDir(), "not-created-yet", "docs")
	if _, err := Ensure(src, dest, ModeSymlink); err == nil {
		t.Fatal("Ensure succeeded with no parent directory")
	}
}

func TestCopyDirReportsAnUnreadableFile(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	mustMkdir(t, src)
	secret := filepath.Join(src, "secret.txt")
	mustWrite(t, secret, "hidden\n", 0o000)
	t.Cleanup(func() { os.Chmod(secret, 0o600) })

	if err := CopyDir(src, filepath.Join(t.TempDir(), "dst")); err == nil {
		t.Fatal("CopyDir succeeded despite an unreadable file")
	}
}
