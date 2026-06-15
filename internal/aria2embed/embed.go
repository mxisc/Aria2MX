package aria2embed

import "io/fs"

func RuntimeArchive() ([]byte, error) {
	if len(runtimeArchive) == 0 {
		return nil, fs.ErrNotExist
	}
	data := make([]byte, len(runtimeArchive))
	copy(data, runtimeArchive)
	return data, nil
}
