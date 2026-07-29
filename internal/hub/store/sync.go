package store

import (
	"log"
	"os"
	"path/filepath"
	"time"
)

// SyncFS scans the filesystem under rootDir and upserts all entries into file_index.
// Filesystem is the source of truth.
func SyncFS(db *DB, rootName, rootDir string) error {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return err
	}

	// Ensure root directory exists
	if err := os.MkdirAll(absRoot, 0755); err != nil {
		return err
	}

	count := 0
	err = filepath.Walk(absRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}

		// Compute relative path from rootDir
		relPath, err := filepath.Rel(absRoot, path)
		if err != nil {
			return nil
		}
		if relPath == "." {
			return nil // skip the root itself
		}

		name := info.Name()
		ftype := "file"
		if info.IsDir() {
			ftype = "dir"
		}

		var content *string
		if !info.IsDir() {
			data, readErr := os.ReadFile(path)
			if readErr == nil {
				s := string(data)
				content = &s
			}
		}

		now := time.Now().UTC()
		_, execErr := db.db.Exec(
			`INSERT INTO file_index (root, path, name, type, content, size, modified_at, synced_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(root, path) DO UPDATE SET
			   name = excluded.name,
			   type = excluded.type,
			   content = excluded.content,
			   size = excluded.size,
			   modified_at = excluded.modified_at,
			   synced_at = excluded.synced_at`,
			rootName, relPath, name, ftype, content, info.Size(), info.ModTime().UTC(), now,
		)
		if execErr != nil {
			log.Printf("[sync] upsert error root=%s path=%s: %v", rootName, relPath, execErr)
		} else {
			count++
		}
		return nil
	})

	if err != nil {
		return err
	}

	// Remove stale entries that no longer exist on filesystem
	rows, err := db.db.Query(`SELECT path FROM file_index WHERE root = ?`, rootName)
	if err != nil {
		return err
	}
	defer rows.Close()

	var stalePaths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			continue
		}
		full := filepath.Join(absRoot, p)
		if _, statErr := os.Stat(full); os.IsNotExist(statErr) {
			stalePaths = append(stalePaths, p)
		}
	}

	for _, p := range stalePaths {
		_, _ = db.db.Exec(`DELETE FROM file_index WHERE root = ? AND path = ?`, rootName, p)
	}

	log.Printf("[sync] SyncFS root=%s dir=%s indexed=%d removed=%d", rootName, rootDir, count, len(stalePaths))
	return nil
}

// SyncToFS writes file_index entries back to the filesystem if they don't exist.
// This can recreate files that were deleted from disk but still in the DB.
func SyncToFS(db *DB, rootName, rootDir string) error {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return err
	}

	rows, err := db.db.Query(
		`SELECT path, type, content FROM file_index WHERE root = ? ORDER BY path`,
		rootName,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	restored := 0
	for rows.Next() {
		var relPath, ftype string
		var content *string
		if err := rows.Scan(&relPath, &ftype, &content); err != nil {
			continue
		}

		full := filepath.Join(absRoot, relPath)
		if _, statErr := os.Stat(full); statErr == nil {
			continue // already exists
		}

		if ftype == "dir" {
			if err := os.MkdirAll(full, 0755); err != nil {
				log.Printf("[sync] restore dir error: %v", err)
			} else {
				restored++
			}
		} else if content != nil {
			// Ensure parent dir exists
			if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
				log.Printf("[sync] restore parent dir error: %v", err)
				continue
			}
			if err := os.WriteFile(full, []byte(*content), 0644); err != nil {
				log.Printf("[sync] restore file error: %v", err)
			} else {
				restored++
			}
		}
	}

	if restored > 0 {
		log.Printf("[sync] SyncToFS root=%s restored=%d entries", rootName, restored)
	}
	return nil
}
