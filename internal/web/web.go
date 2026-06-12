package web

import "embed"

//go:embed dist
var dist embed.FS

func Assets() embed.FS {
	return dist
}
