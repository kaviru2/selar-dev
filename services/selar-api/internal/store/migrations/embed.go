// Package migrations embeds the ordered SQL migrations so the migration
// runner binary is self-contained (no repository checkout needed at deploy time).
package migrations

import "embed"

// FS holds every NNN_description.sql file in this directory.
//
//go:embed *.sql
var FS embed.FS
