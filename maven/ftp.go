package maven

import (
	"github.com/Minecrell/SpongeDownloads/httperror"
	"github.com/secsy/goftp"
	"io"
	"net/http"
	"net/url"
)

// Expose Timeout() method of goftp.Error via interface
type ftpError interface {
	goftp.Error
	Timeout() bool
}

func createFTP(url *url.URL) (*ftpRepository, error) {
	var config goftp.Config

	if url.User != nil {
		config.User = url.User.Username()
		config.Password, _ = url.User.Password()
	}

	client, err := goftp.DialConfig(config, url.Host)
	if err != nil {
		return nil, err
	}

	return &ftpRepository{client, url.Path}, nil
}

type ftpRepository struct {
	ftp      *goftp.Client
	basePath string
}

func (repo *ftpRepository) Download(path string, writer io.Writer) error {
	path = repo.basePath + path

	err := repo.ftp.Retrieve(path, writer)
	if err == nil {
		return nil
	}

	code := http.StatusBadGateway

	if ftpErr, ok := err.(ftpError); ok {
		switch {
		case ftpErr.Code() == 550:
			code = http.StatusNotFound
		case ftpErr.Timeout():
			code = http.StatusGatewayTimeout
		case ftpErr.Temporary():
			code = http.StatusServiceUnavailable
		}
	}

	return httperror.New(code, "Failed to download file", err)
}

func (repo *ftpRepository) Upload(path string, reader io.Reader) error {
	path = repo.basePath + path

	repo.createPath(path)

	err := repo.ftp.Store(path, reader)
	if err == nil {
		return nil
	}

	return httperror.New(http.StatusBadGateway, "Failed to upload file", err)
}

func (ftp *ftpRepository) createPath(path string) {
	for i, c := range path {
		if c == '/' {
			// Ignore errors since the directories may already exist
			ftp.ftp.Mkdir(path[:i])
		}
	}
}
