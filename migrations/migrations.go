// Package migrations embeds the SQL schema files so the database package can
// apply them at startup in lexicographic filename order.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
