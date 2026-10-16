//go:build !embedweb

package webui

import "io/fs"

func embeddedAssets() fs.FS { return nil }
