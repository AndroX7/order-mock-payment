package upload

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Storage persists an uploaded file and returns a relative path that can
// be stored in the database. The concrete implementation may write to
// local disk, object storage, etc.
type Storage interface {
	Save(ctx context.Context, file multipart.File, filename string) (string, error)
}

// LocalStorage writes uploads to a date-partitioned directory tree
// rooted at baseDir. Paths returned are relative to baseDir.
type LocalStorage struct {
	baseDir string
}

func NewLocalStorage(baseDir string) *LocalStorage {
	return &LocalStorage{baseDir: baseDir}
}

// Save writes file to baseDir/YYYY/MM/DD/filename and returns the
// relative path (YYYY/MM/DD/filename). filename must not contain path
// separators or "..".
func (s *LocalStorage) Save(_ context.Context, file multipart.File, filename string) (string, error) {
	if !isSafeFilename(filename) {
		return "", ErrInvalidStoragePath
	}

	now := time.Now().UTC()
	rel := filepath.Join(
		fmt.Sprintf("%04d", now.Year()),
		fmt.Sprintf("%02d", int(now.Month())),
		fmt.Sprintf("%02d", now.Day()),
		filename,
	)

	baseAbs, err := filepath.Abs(s.baseDir)
	if err != nil {
		return "", fmt.Errorf("resolve base dir: %w", err)
	}
	fullAbs, err := filepath.Abs(filepath.Join(baseAbs, rel))
	if err != nil {
		return "", fmt.Errorf("resolve full path: %w", err)
	}
	// Defense-in-depth: even though filename is generated server-side,
	// verify the final path stays under baseDir.
	if !strings.HasPrefix(fullAbs, baseAbs+string(filepath.Separator)) {
		return "", ErrInvalidStoragePath
	}

	if err := os.MkdirAll(filepath.Dir(fullAbs), 0o755); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}

	out, err := os.Create(fullAbs)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	// Copy first, then close, then decide.
	_, copyErr := io.Copy(out, file)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(fullAbs)
		return "", fmt.Errorf("write file: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(fullAbs)
		return "", fmt.Errorf("close file: %w", closeErr)
	}
	return rel, nil
}

// isSafeFilename returns true iff filename contains no path separators
// and no ".." segments. Filenames are generated server-side, but the
// storage layer verifies its own inputs.
func isSafeFilename(name string) bool {
	if name == "" {
		return false
	}
	if strings.ContainsAny(name, "/\\") {
		return false
	}
	if strings.Contains(name, "..") {
		return false
	}
	if name != filepath.Clean(name) {
		return false
	}
	return true
}

var _ Storage = (*LocalStorage)(nil)
