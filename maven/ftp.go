package maven

import (
	"github.com/secsy/goftp"
	"io"
	"strings"
)

func CreateFTPUploader(host string, user, password string) (Uploader, error) {
	client, err := goftp.DialConfig(goftp.Config{User: user, Password: password}, host)
	if err != nil {
		return nil, err
	}
	return &ftpUploader{client}, nil
}

type ftpUploader struct {
	client *goftp.Client
}

func (ftp *ftpUploader) Upload(path string, reader io.Reader) error {
	for _, dir := range splitPath(path) {
		// Ignore errors since the directories may already exist
		ftp.client.Mkdir(dir)
	}

	return ftp.client.Store(path, reader)
}

func splitPath(path string) []string {
	n := strings.Count(path, "/")
	result := make([]string, n)

	pos := 0
	for i := 0; i < n; i++ {
		for path[pos] != '/' {
			pos++
		}

		result[i] = path[0:pos]
		pos++
	}

	return result
}
