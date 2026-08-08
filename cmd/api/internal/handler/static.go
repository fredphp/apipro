package handler

import (
	"os"
)

// readStaticFile is a tiny indirection so tests can stub it.
var readStaticFile = func(path string) ([]byte, error) {
	return os.ReadFile(path)
}
