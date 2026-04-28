// Package state — snapshot.go provides the save/restore snapshot mechanism.
// A snapshot is a full copy of a user's exercise working directory stored at
// ~/.thoth-go/snapshots/<exercise-id>/, allowing the learner to save progress
// and roll back to any previously snapshotted state.
package state

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// DefaultSnapshotDir returns the canonical root directory for all snapshots:
// ~/.thoth-go/snapshots.
func DefaultSnapshotDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".thoth-go", "snapshots"), nil
}

// SaveSnapshot copies every file from srcDir into snapshotRoot/<exerciseID>/,
// replacing any previously saved snapshot for that exercise.
//
// Hidden files and directories are included; only ".git" directories are
// skipped to avoid storing repository metadata.
func SaveSnapshot(exerciseID, srcDir, snapshotRoot string) error {
	if exerciseID == "" {
		return fmt.Errorf("snapshot save: exerciseID must not be empty")
	}
	dst := filepath.Join(snapshotRoot, exerciseID)
	// Wipe the previous snapshot so stale files don't linger.
	if err := os.RemoveAll(dst); err != nil {
		return fmt.Errorf("snapshot save: clearing old snapshot: %w", err)
	}
	if err := copyDir(srcDir, dst); err != nil {
		return fmt.Errorf("snapshot save %q → %q: %w", srcDir, dst, err)
	}
	return nil
}

// LoadSnapshot restores files from snapshotRoot/<exerciseID>/ into dstDir,
// overwriting any conflicting files that already exist there.
//
// Returns an error if no snapshot for exerciseID has been saved yet.
func LoadSnapshot(exerciseID, snapshotRoot, dstDir string) error {
	if exerciseID == "" {
		return fmt.Errorf("snapshot load: exerciseID must not be empty")
	}
	src := filepath.Join(snapshotRoot, exerciseID)
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return fmt.Errorf("snapshot load: no snapshot found for exercise %q", exerciseID)
	}
	if err := copyDir(src, dstDir); err != nil {
		return fmt.Errorf("snapshot load %q → %q: %w", src, dstDir, err)
	}
	return nil
}

// copyDir recursively copies all files from src to dst.
// It creates dst (and any intermediate directories) as needed.
// Directories named ".git" are skipped entirely.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip .git directories.
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		return copyFile(path, target, info.Mode())
	})
}

// copyFile copies a single regular file from src to dst, preserving its
// permission bits.
func copyFile(src, dst string, mode os.FileMode) error {
	// Ensure the parent directory exists.
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
