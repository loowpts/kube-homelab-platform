// Package migrations embeds the SQL migration files so they ship inside the
// compiled binary and can run in a scratch/distroless container without a
// separate volume or config map for the .sql files.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
