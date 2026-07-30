package assets

import "embed"

// FS contains the runtime assets shipped inside the zbrain binary.
//
//go:embed README.md agents/* engine/* skills/* templates/* all:workspaces
var FS embed.FS
