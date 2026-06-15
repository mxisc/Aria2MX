//go:build allinone && linux && amd64

package aria2embed

import _ "embed"

//go:embed runtime/linux-amd64.tar.gz
var runtimeArchive []byte
