package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func EnsureSessionDir(baseDir, runID string) (string, error) {
	dir := filepath.Join(baseDir, fmt.Sprintf("session-%s", runID))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create session dir: %w", err)
	}
	return dir, nil
}

func CleanExpiredSessions(baseDir string, maxAge time.Duration) (int, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	removed := 0
	now := time.Now()
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > maxAge {
			dir := filepath.Join(baseDir, e.Name())
			if err := os.RemoveAll(dir); err == nil {
				removed++
			}
		}
	}
	return removed, nil
}
