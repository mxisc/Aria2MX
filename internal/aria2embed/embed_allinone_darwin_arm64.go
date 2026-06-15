//go:build allinone && darwin && arm64

package aria2embed

import _ "embed"

//go:embed runtime/darwin-arm64.tar.gz
var runtimeArchive []byte
