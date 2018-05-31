package maven

import (
	"github.com/SpongePowered/DownloadIndexer/httperror"
	"github.com/secsy/goftp"
	"io"
	"net/http"
	"net/url"
	"os"
)

// Expose Timeout() method of goftp.Error via interface
type ftpError interface {
	goftp.Error
	Timeout() bool
}

func createFTP(url *url.URL) (*ftpRepository, error) {
	config := goftp.Config{
		ConnectionsPerHost: 3,
	}

	if os.Getenv("FTP_DEBUG") != "" {
		config.Logger = os.Stdout
	}

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

	// Retrieve can fail after the FTP connection has not been used for a while (due to timeouts)
	// To prevent this, we attempt retrieving it for 3 times before failing the request
	var err error
	for i := 0; i < 3; i++ {
		err = repo.ftp.Retrieve(path, writer)
		if err == nil {
			return nil
		}

		continue
		/*if ftpErr, ok := err.(ftpError); ok && ftpErr.Temporary() && strings.HasPrefix(ftpErr.Message(), "Timeout") {
			continue
		}

		// We can stop trying because the error is not recoverable
		break*/
	}

	code := http.StatusBadGateway

	if ftpErr, ok := err.(ftpError); ok {
		switch {
		case ftpErr.Code() == 550:
			code = http.StatusNotFound
		case ftpErr.Timeout():
			code = http.StatusGatewayTimeout
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

func (repo *ftpRepository) createPath(path string) {
	for i, c := range path {
		if i > 0 && c == '/' {
			// Ignore errors since the directories may already exist
			repo.ftp.Mkdir(path[:i])
		}
	}
}
