package migration

import "embed"

// Force Go to include SQL files
//
//go:embed *.sql
var FS embed.FS
