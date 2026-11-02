// Package migrations embeds the SQL migration files so that the migrate
// binary is fully self-contained and does not depend on the filesystem layout
// at runtime.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
