package maven

import (
	"github.com/Minecrell/SpongeDownloads/downloads"
	"io"
)

type nullRepository struct{}

func (r nullRepository) Download(path string, writer io.Writer) error {
	return downloads.NotFound(path + " does not exist")
}

func (r nullRepository) Upload(path string, reader io.Reader) error {
	return nil // Ignore upload
}
