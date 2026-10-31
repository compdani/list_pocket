package plugins

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	maxArchiveEntries = 2000
	maxArchiveBytes   = 50 << 20
	maxArchiveFile    = 20 << 20
)

// extractArchive unpacks a zip into dest, which must already exist. It is written
// against the bytes actually read, not the sizes the archive claims, and every
// write goes through os.Root so nothing can land outside dest.
func extractArchive(zr *zip.Reader, dest string) error {
	if len(zr.File) > maxArchiveEntries {
		return fmt.Errorf("archive has more than %d entries", maxArchiveEntries)
	}

	root, err := os.OpenRoot(dest)
	if err != nil {
		return err
	}
	defer root.Close()

	var total int64
	for _, f := range zr.File {
		name := strings.TrimSuffix(f.Name, "/")
		if name == "" || !isLocalPath(name) {
			return fmt.Errorf("archive entry %q has an unsafe path", f.Name)
		}
		mode := f.Mode()
		if mode.IsDir() {
			if err := root.MkdirAll(filepath.FromSlash(name), 0o755); err != nil {
				return err
			}
			continue
		}
		if !mode.IsRegular() {
			return fmt.Errorf("archive entry %q is not a regular file", f.Name)
		}
		if dir := path.Dir(name); dir != "." {
			if err := root.MkdirAll(filepath.FromSlash(dir), 0o755); err != nil {
				return err
			}
		}
		n, err := extractEntry(root, f, name, maxArchiveBytes-total)
		if err != nil {
			return err
		}
		total += n
	}
	return nil
}

func extractEntry(root *os.Root, f *zip.File, name string, remaining int64) (int64, error) {
	limit := min(int64(maxArchiveFile), remaining)

	rc, err := f.Open()
	if err != nil {
		return 0, err
	}
	defer rc.Close()

	out, err := root.OpenFile(filepath.FromSlash(name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(out, io.LimitReader(rc, limit+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return n, err
	}
	if n > limit {
		return n, errors.New("archive exceeds the size limit")
	}
	return n, nil
}

// archiveRoot returns the directory holding plugin.json, allowing for archives
// that wrap everything in a single top-level folder.
func archiveRoot(dir string) (string, error) {
	if _, err := os.Stat(filepath.Join(dir, manifestFile)); err == nil {
		return dir, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	if len(entries) == 1 && entries[0].IsDir() {
		sub := filepath.Join(dir, entries[0].Name())
		if _, err := os.Stat(filepath.Join(sub, manifestFile)); err == nil {
			return sub, nil
		}
	}
	return "", fmt.Errorf("archive has no %s", manifestFile)
}
