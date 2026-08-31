// Moving a skill directory across filesystems: the canonical store and the
// fleet repo rarely share a mount, so a plain rename can fail with EXDEV.
// The fallback copies the tree (regular files with their modes, nested
// directories, symlinks recreated verbatim) and only then deletes the
// source, so a failed copy never loses the skill.

package customs

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// moveDir renames src to dst, falling back to a copy+delete when the two
// live on different filesystems. dst must not exist.
func moveDir(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}
	if !errors.Is(err, syscall.EXDEV) {
		return err
	}
	if err := copyTree(src, dst); err != nil {
		// A partial copy is never kept, and the source is still intact.
		_ = os.RemoveAll(dst)
		return err
	}
	return os.RemoveAll(src)
}

// copyTree recreates the src directory tree at dst.
func copyTree(src, dst string) error {
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
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case d.Type().IsRegular():
			return copyFile(path, target)
		default:
			// Sockets, devices, and other exotica are not skill content;
			// leave them behind rather than fail the move.
			return nil
		}
	})
}

func copyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }() // read-only: nothing to propagate

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close() // the copy error is the one that matters
		return err
	}
	return out.Close()
}
