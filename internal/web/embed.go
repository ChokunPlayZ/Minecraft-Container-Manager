// Package web embeds the built static frontend for serving alongside the API.
package web

import "embed"

//go:embed dist
var Dist embed.FS
