// Package resources contains the desktop application's visual assets.
package resources

import _ "embed"

// IconPNG is embedded so directly launching the binary also has a window icon.
//
//go:embed icons/png/512.png
var IconPNG []byte
