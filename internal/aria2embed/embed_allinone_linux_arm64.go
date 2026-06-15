//go:build allinone && linux && arm64

package aria2embed

import _ "embed"

//go:embed runtime/linux-arm64.tar.gz
var runtimeArchive []byte
