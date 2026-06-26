package server

import (
	"embed"
	"io/fs"
)

// webuiDist holds the production build of the Vite SPA (the /panel UI).
// Run `make ui-build` (or `cd ui && npm run build`) to populate dist/ before
// building the server binary.
//
//go:embed all:webui/dist
var webuiDist embed.FS

// webuiFS returns the SPA build rooted at the dist directory.
func webuiFS() fs.FS {
	sub, err := fs.Sub(webuiDist, "webui/dist")
	if err != nil {
		panic(err)
	}
	return sub
}
