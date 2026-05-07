// Package web ships the embedded SPA bundle that the flow HTTP server serves.
//
// The Next.js app under this directory builds a static export into ./out
// (Next's default for `output: "export"`). At Go build time we embed the
// contents of ./out as an io/fs.FS via Dist().
//
// To compile this package the out/ directory must exist and contain at least
// one file. Run `cd web && npm install && npm run build` before `go build`.
// A placeholder is committed so a fresh checkout compiles without requiring
// the npm build first; running the npm build populates the directory.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:out
var distFS embed.FS

// Dist returns the embedded SPA bundle rooted at the out/ directory.
// Returns nil if the bundle is empty (unbuilt) — the API server then renders
// a small "frontend not built" notice.
func Dist() fs.FS {
	sub, err := fs.Sub(distFS, "out")
	if err != nil {
		return nil
	}
	// Treat an empty bundle (only the placeholder marker) as nil so the
	// server falls back to the no-bundle notice instead of serving a stub.
	if isEmpty(sub) {
		return nil
	}
	return sub
}

func isEmpty(fsys fs.FS) bool {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return true
	}
	for _, e := range entries {
		if e.Name() == ".gitkeep" {
			continue
		}
		return false
	}
	return true
}
