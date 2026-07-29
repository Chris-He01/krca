package store

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"knsight-go/internal/hub/user"
)

// FileNode represents a file or directory in the tree.
type FileNode struct {
	Path     string      `json:"path"`
	Name     string      `json:"name"`
	Type     string      `json:"type"` // "file" or "dir"
	Size     int64       `json:"size,omitempty"`
	Modified *time.Time  `json:"modified,omitempty"`
	Children []*FileNode `json:"children,omitempty"`
}

// FileTreeAPI provides HTTP handlers for browsing configured directories.
type FileTreeAPI struct {
	db    *DB
	roots map[string]string // root name -> absolute directory path

	// OnWrite is invoked after a successful local file write (handleWrite).
	// Implementations should be best-effort and must not block the request meaningfully.
	// Typical use: persist user edits to Redis so they survive container/image restarts.
	OnWrite func(root, relPath, content string)
	// OnDelete is invoked after a successful local file/dir delete (handleDelete).
	OnDelete func(root, relPath string)
}

// NewFileTreeAPI creates a new FileTreeAPI with the given root mappings.
func NewFileTreeAPI(db *DB, roots map[string]string) *FileTreeAPI {
	// Resolve all roots to absolute paths
	resolved := make(map[string]string, len(roots))
	for name, dir := range roots {
		abs, err := filepath.Abs(dir)
		if err != nil {
			log.Printf("[filetree] warning: cannot resolve root %q=%q: %v", name, dir, err)
			continue
		}
		resolved[name] = abs
	}
	return &FileTreeAPI{db: db, roots: resolved}
}

// resolveRoot returns the absolute directory for a root name.
// Returns an error if the root name is not configured.
func (a *FileTreeAPI) resolveRoot(rootName string) (string, error) {
	dir, ok := a.roots[rootName]
	if !ok {
		return "", fmt.Errorf("unknown root: %q", rootName)
	}
	return dir, nil
}

// safePath validates that the given relative path does not escape the root directory.
// Returns the cleaned absolute path.
func (a *FileTreeAPI) safePath(rootDir, relPath string) (string, error) {
	// Clean and join
	cleaned := filepath.Clean(relPath)
	if cleaned == "." {
		return rootDir, nil
	}
	// Reject absolute paths and path traversal
	if filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, "..") {
		return "", fmt.Errorf("invalid path: %q", relPath)
	}
	full := filepath.Join(rootDir, cleaned)
	// Double-check the resolved path is within root
	if !strings.HasPrefix(full, rootDir+string(filepath.Separator)) && full != rootDir {
		return "", fmt.Errorf("path escapes root: %q", relPath)
	}
	return full, nil
}

// checkUserAccess verifies that the user is allowed to access the given relative path.
// Paths under "user/{username}" are only accessible to that user.
func checkUserAccess(relPath, userID string) bool {
	cleaned := filepath.Clean(relPath)
	parts := strings.SplitN(cleaned, string(filepath.Separator), 3)
	if len(parts) >= 2 && parts[0] == "user" && parts[1] != userID {
		return false
	}
	return true
}

// buildTree recursively walks a directory and builds a FileNode tree.
// userID is used to filter user-scoped directories: under a "user/" directory,
// only the subdirectory matching userID is shown.
func buildTree(rootName, rootDir, dir, userID string) (*FileNode, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(rootDir, dir)
	isRoot := relPath == "."

	name := info.Name()
	if isRoot {
		relPath = ""
		name = rootName
	}

	node := &FileNode{
		Path: relPath,
		Name: name,
		Type: "dir",
	}

	if !info.IsDir() {
		node.Type = "file"
		node.Size = info.Size()
		mod := info.ModTime().UTC()
		node.Modified = &mod
		return node, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	// If this directory is named "user", filter children to only show the current user's dir
	isUserDir := name == "user" && relPath == "user"
	for _, entry := range entries {
		if isUserDir && entry.IsDir() && entry.Name() != userID {
			continue
		}
		childPath := filepath.Join(dir, entry.Name())
		child, err := buildTree(rootName, rootDir, childPath, userID)
		if err != nil {
			// Skip entries we cannot read
			continue
		}
		node.Children = append(node.Children, child)
	}

	if node.Children == nil {
		node.Children = []*FileNode{}
	}

	return node, nil
}

// Handle is a combined handler for /v1/filetree/ that dispatches based on sub-path.
// Handles tree listing (no sub-path or just "/") and CRUD operations.
func (a *FileTreeAPI) Handle() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		op := strings.TrimPrefix(r.URL.Path, "/v1/filetree")
		op = strings.TrimPrefix(op, "/")

		switch op {
		case "":
			a.handleTree(w, r)
		case "read":
			a.handleRead(w, r)
		case "write":
			a.handleWrite(w, r)
		case "mkdir":
			a.handleMkdir(w, r)
		case "delete":
			a.handleDelete(w, r)
		default:
			http.NotFound(w, r)
		}
	}
}

// handleTree handles GET /v1/filetree?root=skills
func (a *FileTreeAPI) handleTree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rootName := r.URL.Query().Get("root")
	rootDir, err := a.resolveRoot(rootName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Ensure the directory exists
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		http.Error(w, fmt.Sprintf("cannot access root: %v", err), http.StatusInternalServerError)
		return
	}

	u := user.FromContext(r.Context())
	tree, err := buildTree(rootName, rootDir, rootDir, u.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("scan error: %v", err), http.StatusInternalServerError)
		return
	}

	writeFileTreeJSON(w, tree)
}

// handleRead handles GET /v1/filetree/read?root=skills&path=_common/example/SKILL.md
func (a *FileTreeAPI) handleRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rootName := r.URL.Query().Get("root")
	rootDir, err := a.resolveRoot(rootName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	relPath := r.URL.Query().Get("path")
	if relPath == "" {
		http.Error(w, "missing path parameter", http.StatusBadRequest)
		return
	}

	u := user.FromContext(r.Context())
	if !checkUserAccess(relPath, u.ID) {
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}

	fullPath, err := a.safePath(rootDir, relPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "file not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeFileTreeJSON(w, map[string]string{
		"path":    relPath,
		"content": string(data),
	})
}

// handleWrite handles POST /v1/filetree/write
// Body: {"root": "skills", "path": "scope/name/SKILL.md", "content": "..."}
func (a *FileTreeAPI) handleWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Root    string `json:"root"`
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	rootDir, err := a.resolveRoot(body.Root)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Path == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}

	u := user.FromContext(r.Context())
	if !checkUserAccess(body.Path, u.ID) {
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}

	fullPath, err := a.safePath(rootDir, body.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(fullPath, []byte(body.Content), 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Update SQLite index
	info, _ := os.Stat(fullPath)
	if info != nil {
		a.upsertFileIndex(body.Root, body.Path, info)
	}
	// Also index parent directories up to root
	a.indexParentDirs(body.Root, body.Path)

	// Invoke optional persistence hook (e.g. mirror to Redis) so user edits
	// survive container restarts where the local rootfs is reset to the image layer.
	if a.OnWrite != nil {
		a.OnWrite(body.Root, filepath.ToSlash(filepath.Clean(body.Path)), body.Content)
	}

	writeFileTreeJSON(w, map[string]string{"status": "ok", "path": body.Path})
}

// handleMkdir handles POST /v1/filetree/mkdir
// Body: {"root": "skills", "path": "newscope/newskill"}
func (a *FileTreeAPI) handleMkdir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Root string `json:"root"`
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	rootDir, err := a.resolveRoot(body.Root)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Path == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}

	u := user.FromContext(r.Context())
	if !checkUserAccess(body.Path, u.ID) {
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}

	fullPath, err := a.safePath(rootDir, body.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := os.MkdirAll(fullPath, 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Update SQLite index for the dir and parents
	a.upsertDirIndex(body.Root, body.Path)
	a.indexParentDirs(body.Root, body.Path)

	writeFileTreeJSON(w, map[string]string{"status": "ok", "path": body.Path})
}

// handleDelete handles DELETE /v1/filetree/delete
// Body: {"root": "skills", "path": "test/111"}
func (a *FileTreeAPI) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Root string `json:"root"`
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	rootDir, err := a.resolveRoot(body.Root)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Path == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}

	u := user.FromContext(r.Context())
	if !checkUserAccess(body.Path, u.ID) {
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}

	fullPath, err := a.safePath(rootDir, body.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Don't allow deleting the root itself
	if fullPath == rootDir {
		http.Error(w, "cannot delete root directory", http.StatusBadRequest)
		return
	}

	if err := os.RemoveAll(fullPath); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Remove from SQLite index (the path and everything under it)
	a.deleteFileIndex(body.Root, body.Path)

	// Invoke optional persistence hook (e.g. mirror delete to Redis).
	if a.OnDelete != nil {
		a.OnDelete(body.Root, filepath.ToSlash(filepath.Clean(body.Path)))
	}

	writeFileTreeJSON(w, map[string]string{"status": "ok", "path": body.Path})
}

// upsertFileIndex inserts or updates a file entry in the file_index table.
func (a *FileTreeAPI) upsertFileIndex(root, relPath string, info fs.FileInfo) {
	name := filepath.Base(relPath)
	ftype := "file"
	if info.IsDir() {
		ftype = "dir"
	}
	var content *string
	if !info.IsDir() {
		data, err := os.ReadFile(filepath.Join(a.roots[root], relPath))
		if err == nil {
			s := string(data)
			content = &s
		}
	}
	_, err := a.db.db.Exec(
		`INSERT INTO file_index (root, path, name, type, content, size, modified_at, synced_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(root, path) DO UPDATE SET
		   name = excluded.name,
		   type = excluded.type,
		   content = excluded.content,
		   size = excluded.size,
		   modified_at = excluded.modified_at,
		   synced_at = excluded.synced_at`,
		root, relPath, name, ftype, content, info.Size(), info.ModTime().UTC(), time.Now().UTC(),
	)
	if err != nil {
		log.Printf("[filetree] upsert index error: %v", err)
	}
}

// upsertDirIndex inserts a directory entry.
func (a *FileTreeAPI) upsertDirIndex(root, relPath string) {
	name := filepath.Base(relPath)
	_, err := a.db.db.Exec(
		`INSERT INTO file_index (root, path, name, type, size, modified_at, synced_at)
		 VALUES (?, ?, ?, 'dir', 0, ?, ?)
		 ON CONFLICT(root, path) DO UPDATE SET
		   name = excluded.name,
		   type = excluded.type,
		   synced_at = excluded.synced_at`,
		root, relPath, name, time.Now().UTC(), time.Now().UTC(),
	)
	if err != nil {
		log.Printf("[filetree] upsert dir index error: %v", err)
	}
}

// indexParentDirs ensures all parent directories of a path are indexed.
func (a *FileTreeAPI) indexParentDirs(root, relPath string) {
	dir := filepath.Dir(relPath)
	for dir != "." && dir != "/" {
		a.upsertDirIndex(root, dir)
		dir = filepath.Dir(dir)
	}
}

// deleteFileIndex removes entries from file_index for the given path and everything beneath it.
func (a *FileTreeAPI) deleteFileIndex(root, relPath string) {
	// Delete exact match and anything under it
	_, err := a.db.db.Exec(
		`DELETE FROM file_index WHERE root = ? AND (path = ? OR path LIKE ?)`,
		root, relPath, relPath+"/%",
	)
	if err != nil {
		log.Printf("[filetree] delete index error: %v", err)
	}
}

func writeFileTreeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}
