package indexer

import (
	"io"
	"strings"
)

type Indexer struct {
	Target string
}

func (i *Indexer) Upload(path string, reader io.Reader) (err error) {
	// TODO: Better way to detect artifacts?
	if !strings.HasSuffix(path, ".jar") {
		return
	}

	return
}
