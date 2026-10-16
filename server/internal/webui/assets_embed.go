//go:build embedweb

package webui

import (
	"embed"
	"io/fs"
)

// Build with task build so dist contains the current production frontend.
//
//go:embed dist
var bundled embed.FS

func embeddedAssets() fs.FS {
	assets, err := fs.Sub(bundled, "dist")
	if err != nil {
		panic(err)
	}
	return assets
}
