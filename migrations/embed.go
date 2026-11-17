// Package migrations embeds the Goose SQL migrations into the permitpal binary.
package migrations

import "embed"

// Files exposes all SQL migrations embedded into the binary.
//
//go:embed *.sql
var Files embed.FS
