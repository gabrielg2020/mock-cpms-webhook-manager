package web

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"strings"
)

// assetVersion is a short hash of every static file. It goes in the URL, so the address of
// every asset changes whenever any of them does, and browsers can cache them indefinitely.
func assetVersion(static fs.FS) (string, error) {
	hash := sha256.New()
	err := fs.WalkDir(static, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		contents, err := fs.ReadFile(static, name)
		if err != nil {
			return err
		}
		hash.Write([]byte(name))
		hash.Write(contents)
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil))[:12], nil
}

func (app *Application) staticFiles() http.Handler {
	files := http.StripPrefix(app.staticPath+"/", http.FileServerFS(app.static))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The file server would otherwise answer a folder path with a list of its files.
		if strings.HasSuffix(r.URL.Path, "/") {
			app.notFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		files.ServeHTTP(w, r)
	})
}
