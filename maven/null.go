package maven

import (
	"github.com/SpongePowered/DownloadIndexer/httperror"
	"io"
)

type nullRepository struct{}

func (r nullRepository) Download(path string, writer io.Writer) error {
	return httperror.NotFound(path + " does not exist")
}

func (r nullRepository) Upload(path string, reader io.Reader, _ int64) error {
	return nil // Ignore upload
}
