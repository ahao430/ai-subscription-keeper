// Package webui embeds the built frontend (web/dist copied here by the build
// pipeline). When only the placeholder exists the server serves a hint page.
package webui

import (
	"embed"
	"io/fs"

	// Register the dist directory; .gitkeep keeps the embed valid before the
	// first frontend build.
	_ "embed"
)

//go:embed all:dist
var distFS embed.FS

// Dist is the embedded frontend filesystem rooted at "dist".
var Dist fs.FS = distFS
