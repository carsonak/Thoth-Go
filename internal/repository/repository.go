// Package repository manages fetching, caching, and resetting exercise topic
// packages for thoth-go.
//
// Remote layout (served by the exercise server):
//
//	GET {BaseURL}/topics/{topic}.zip → zip archive of the topic directory
//
// Local cache layout:
//
//	~/.thoth-go/cache/topics/{topic}/{exercise-id}/exercise.yaml
//	~/.thoth-go/cache/topics/{topic}/{exercise-id}/*.go
//
// Each topic is downloaded once and extracted into the local cache. Subsequent
// calls to Fetch are no-ops unless force=true is passed.
package repository

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Manager handles fetching and caching exercise topic packages.
type Manager struct {
	// CacheDir is the root directory for all cached topics,
	// e.g. ~/.thoth-go/cache/topics.
	CacheDir string
	// BaseURL is the root URL of the exercise server,
	// e.g. "https://exercises.thoth-go.dev".
	BaseURL string
	// client is the HTTP client used for downloads; injectable for testing.
	client *http.Client
}

// NewManager creates a Manager with the given cache directory and base URL.
// A nil client falls back to http.DefaultClient.
func NewManager(cacheDir, baseURL string, client *http.Client) *Manager {
	if client == nil {
		client = http.DefaultClient
	}
	return &Manager{
		CacheDir: cacheDir,
		BaseURL:  baseURL,
		client:   client,
	}
}

// DefaultCacheDir returns the canonical cache directory:
// ~/.thoth-go/cache/topics.
func DefaultCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".thoth-go", "cache", "topics"), nil
}

// Fetch downloads the zip bundle for topic and extracts it into
// m.CacheDir/<topic>/.
//
// If the topic directory already exists and force is false, Fetch is a no-op
// (cache hit). With force=true the cache entry is deleted and re-downloaded.
func (m *Manager) Fetch(topic string, force bool) error {
	if topic == "" {
		return fmt.Errorf("fetch: topic must not be empty")
	}

	topicDir := filepath.Join(m.CacheDir, topic)

	// Cache hit — skip unless force is requested.
	if !force {
		if _, err := os.Stat(topicDir); err == nil {
			return nil
		}
	}

	// Build the download URL: {BaseURL}/topics/{topic}.zip
	url := strings.TrimRight(m.BaseURL, "/") + "/topics/" + topic + ".zip"

	resp, err := m.client.Get(url) //nolint:noctx // intentionally simple for CLI use
	if err != nil {
		return fmt.Errorf("fetch %q: HTTP GET: %w", topic, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch %q: server returned %d", topic, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("fetch %q: reading response body: %w", topic, err)
	}

	// Wipe the existing cached directory (covers the force=true case).
	if err := os.RemoveAll(topicDir); err != nil {
		return fmt.Errorf("fetch %q: clearing cache: %w", topic, err)
	}

	if err := extractZip(body, topicDir); err != nil {
		return fmt.Errorf("fetch %q: extracting zip: %w", topic, err)
	}

	return nil
}

// Reset copies the pristine cached files for exerciseID into dstDir.
//
// The method scans all topic subdirectories of m.CacheDir looking for a
// directory whose name matches exerciseID. Returns an error if the exercise
// is not found in the cache (run `thoth-go fetch <topic>` first).
func (m *Manager) Reset(exerciseID, dstDir string) error {
	if exerciseID == "" {
		return fmt.Errorf("reset: exerciseID must not be empty")
	}

	cached, err := m.findExercise(exerciseID)
	if err != nil {
		return err
	}

	if err := copyDir(cached, dstDir); err != nil {
		return fmt.Errorf("reset %q → %q: %w", cached, dstDir, err)
	}
	return nil
}

// findExercise scans the cache for a directory matching exerciseID and returns
// its absolute path.
func (m *Manager) findExercise(exerciseID string) (string, error) {
	// List topic directories.
	entries, err := os.ReadDir(m.CacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("reset: cache directory %q not found; run 'thoth-go fetch <topic>' first", m.CacheDir)
		}
		return "", fmt.Errorf("reset: reading cache: %w", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		candidate := filepath.Join(m.CacheDir, e.Name(), exerciseID)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("reset: exercise %q not found in cache; run 'thoth-go fetch <topic>' first", exerciseID)
}

// extractZip extracts the contents of the zip archive (raw bytes) into dstDir.
//
// Security: file paths are sanitised to prevent zip-slip path traversal.
func extractZip(data []byte, dstDir string) error {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("opening zip: %w", err)
	}

	for _, f := range r.File {
		// Sanitise path to prevent zip-slip.
		rel := filepath.Clean(f.Name)
		if strings.HasPrefix(rel, "..") {
			return fmt.Errorf("zip entry %q escapes destination directory", f.Name)
		}

		target := filepath.Join(dstDir, rel)

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, f.Mode()); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}

		if err := extractZipFile(f, target); err != nil {
			return err
		}
	}
	return nil
}

// extractZipFile writes a single zip entry to dst.
func extractZipFile(f *zip.File, dst string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, rc); err != nil { //nolint:gosec // zip entry size is validated indirectly by memory limits
		return err
	}
	return out.Close()
}

// copyDir recursively copies src → dst, skipping ".git" directories.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
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

// copyFile copies a single file from src to dst, preserving its permission bits.
func copyFile(src, dst string, mode os.FileMode) error {
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
