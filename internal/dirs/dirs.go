// Package dirs materializes plain (non-git) directories inside a grove.
//
// Worktrees are the right tool for repositories, but a grove often also wants
// a directory that has no git history at all — reference docs, a scratch
// example, a vendored SDK. Those are copied in by default: a grove is
// disposable, so the copy can be pruned with everything else and nothing done
// inside the grove can reach the original. A symlink is available for the
// cases where edits should land in the source directory itself.
package dirs

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Modes a directory can be materialized with.
const (
	ModeSymlink = "symlink"
	ModeCopy    = "copy"
)

// ValidMode reports whether mode is one Ensure understands.
func ValidMode(mode string) bool {
	return mode == ModeSymlink || mode == ModeCopy
}

// State describes what is present at a directory's destination in a grove.
type State struct {
	Present   bool
	IsSymlink bool
	Target    string // absolute symlink target; set only when IsSymlink
}

// Inspect reports what exists at dest. It does not follow a final symlink:
// whether the destination *is* a link, and where it points, is exactly what
// callers need to tell an intact link from a stale one.
func Inspect(dest string) (State, error) {
	fi, err := os.Lstat(dest)
	if os.IsNotExist(err) {
		return State{}, nil
	}
	if err != nil {
		return State{}, err
	}
	s := State{Present: true, IsSymlink: fi.Mode()&os.ModeSymlink != 0}
	if !s.IsSymlink {
		return s, nil
	}
	target, err := os.Readlink(dest)
	if err != nil {
		return s, err
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(dest), target)
	}
	s.Target = filepath.Clean(target)
	return s, nil
}

// Ensure materializes source at dest using mode and reports what it did. An
// empty action means dest was already in the desired state.
//
// A copy is made once and then left alone — the grove owns that directory, and
// re-running sync must not throw away work done inside it. A symlink pointing
// somewhere other than source is retargeted, mirroring the way sync fixes
// branch drift for repos. A directory whose mode changed is handled too: a
// leftover symlink is replaced by the copy, while a real directory is never
// deleted out from under you.
func Ensure(source, dest, mode string) (string, error) {
	st, err := Inspect(dest)
	if err != nil {
		return "", fmt.Errorf("could not inspect %s: %w", dest, err)
	}

	switch mode {
	case ModeCopy:
		if st.Present && !st.IsSymlink {
			return "", nil
		}
		if st.IsSymlink {
			// Left over from symlink mode. Unlinking cannot touch whatever
			// it points at, so replacing it with the copy is safe.
			if err := os.Remove(dest); err != nil {
				return "", fmt.Errorf("could not replace symlink %s: %w", dest, err)
			}
		}
		if err := CopyDir(source, dest); err != nil {
			return "", err
		}
		return fmt.Sprintf("copied %s → %s", source, dest), nil

	case ModeSymlink:
		want := filepath.Clean(source)
		if st.Present {
			if !st.IsSymlink {
				return "", fmt.Errorf("%s already exists as a real directory — delete it (gitgrove remove %s --prune) or add it under a different name", dest, filepath.Base(dest))
			}
			if st.Target == want {
				return "", nil
			}
			if err := os.Remove(dest); err != nil {
				return "", fmt.Errorf("could not replace stale symlink %s: %w", dest, err)
			}
			if err := os.Symlink(want, dest); err != nil {
				return "", fmt.Errorf("could not symlink %s → %s: %w", dest, want, err)
			}
			return fmt.Sprintf("retargeted symlink %s → %s", dest, want), nil
		}
		if err := os.Symlink(want, dest); err != nil {
			return "", fmt.Errorf("could not symlink %s → %s: %w", dest, want, err)
		}
		return fmt.Sprintf("symlinked %s → %s", dest, want), nil
	}

	return "", fmt.Errorf("unknown mode %q (want %q or %q)", mode, ModeCopy, ModeSymlink)
}

// Remove deletes what sync put at dest.
//
// A symlink is unlinked, which never touches the directory it points at. A
// copy is deleted outright — it is grove-owned scratch, and the source is
// untouched either way. A real directory sitting where a symlink was expected
// is not something gitgrove created, so it is left in place unless force says
// otherwise.
func Remove(dest, mode string, force bool) error {
	st, err := Inspect(dest)
	if err != nil {
		return err
	}
	if !st.Present {
		return nil
	}
	if st.IsSymlink {
		return os.Remove(dest)
	}
	if mode != ModeCopy && !force {
		return fmt.Errorf("%s is a real directory, not something gitgrove created — pass --force to delete it", dest)
	}
	return os.RemoveAll(dest)
}

// CopyDir recursively copies src to dst. Symlinks inside the tree are
// recreated rather than followed, so a directory full of links does not get
// silently expanded into a much larger copy.
func CopyDir(src, dst string) error {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)
	if dst == src || strings.HasPrefix(dst, src+string(os.PathSeparator)) {
		return fmt.Errorf("cannot copy %s into itself", src)
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			info, err := d.Info()
			if err != nil {
				return err
			}
			return os.MkdirAll(target, info.Mode().Perm())
		case d.Type()&fs.ModeSymlink != 0:
			dest, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(dest, target)
		case d.Type().IsRegular():
			return copyFile(path, target, d)
		default:
			// Sockets, devices and fifos have nothing meaningful to copy.
			return nil
		}
	})
}

func copyFile(src, dst string, d fs.DirEntry) error {
	info, err := d.Info()
	if err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
