//go:build !allinone || (allinone && !(darwin && arm64) && !(linux && amd64) && !(linux && arm64))

package aria2embed

var runtimeArchive []byte
