// Package ui embeds the compiled frontend assets into the Go binary.
package ui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist
var assets embed.FS

// DistFS returns a filesystem rooted at the dist/ directory.
func DistFS() (http.FileSystem, error) {
	sub, err := fs.Sub(assets, "dist")
	if err != nil {
		return nil, err
	}
	return http.FS(sub), nil
}
