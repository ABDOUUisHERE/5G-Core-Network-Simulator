package web

import "embed"

// Assets contains the dashboard without a Node toolchain.
//
//go:embed index.html app.js style.css
var Assets embed.FS
