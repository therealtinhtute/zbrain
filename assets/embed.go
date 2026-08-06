package assets

import "embed"

// FS contains the runtime assets shipped inside the zbrain binary.
//
//go:embed README.md agents/* engine/* skills/* templates/* workspaces/.gitkeep
var FS embed.FS
