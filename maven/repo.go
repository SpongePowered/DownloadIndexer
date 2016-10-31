package maven

import (
	"errors"
	"io"
	"net/url"
)

type Repository interface {
	Download(path string, writer io.Writer) error
	Upload(path string, reader io.Reader) error
}

func CreateRepository(urlString string) (Repository, error) {
	u, err := url.Parse(urlString)
	if err != nil {
		return nil, err
	}

	switch u.Scheme {
	case "http", "https":
		return createHTTP(u)
	case "ftp":
		return createFTP(u)
	case "null":
		return nullRepository{}, nil
	default:
		return nil, errors.New("Unsupported repository format: " + u.Scheme)
	}
}
