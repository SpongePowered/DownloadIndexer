package maven

import (
	"errors"
	"io"
	"net/url"
)

type Repository interface {
	Download(path string, writer io.Writer) error
	Upload(path string, reader io.Reader, len int64) error
}

func CreateRepository(urlString string) (Repository, error) {
	u, err := url.Parse(urlString)
	if err != nil {
		return nil, err
	}

	// Make sure path ends with a slash
	if u.Path != "" && u.Path[len(u.Path)-1] != '/' {
		u.Path += "/"
	}

	switch u.Scheme {
	case "http", "https":
		return createHTTP(u)
	case "file":
		return createFile(u)
	case "ftp":
		return createFTP(u)
	case "null":
		return nullRepository{}, nil
	default:
		return nil, errors.New("Unsupported repository format: " + u.Scheme)
	}
}
