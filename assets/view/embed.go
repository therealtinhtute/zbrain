// Package view embeds the read-only viewer UI served by `zbrain view`.
package view

import "embed"

// FS contains the viewer HTML/CSS/JS served over loopback.
//
//go:embed index.html style.css app.js
var FS embed.FS
