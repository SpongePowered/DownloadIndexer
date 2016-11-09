package maven

import (
	"errors"
	"github.com/Minecrell/SpongeDownloads/downloads"
	"io"
	"net/url"
	"os"
	"path/filepath"
)

func createFile(dir *url.URL) (*fileRepository, error) {
	d, err := os.Stat(dir.Path)
	if err != nil {
		return nil, err
	}

	if !d.IsDir() {
		return nil, errors.New(dir.Path + " is not a directory")
	}

	return &fileRepository{dir.Path}, nil
}

type fileRepository struct {
	dir string
}

func (repo *fileRepository) Download(path string, writer io.Writer) error {
	f, err := os.Open(repo.dir + path)
	if err != nil {
		if os.IsNotExist(err) {
			return downloads.NotFound("File does not exist")
		}

		return downloads.InternalError("Failed to open file", err)
	}

	defer f.Close()

	_, err = io.Copy(writer, f)
	if err != nil {
		return downloads.InternalError("Failed to read file", err)
	}

	return nil
}

func (repo *fileRepository) Upload(path string, reader io.Reader) error {
	path = repo.dir + path

	err := os.MkdirAll(filepath.Dir(path), os.ModePerm)
	if err != nil {
		return downloads.InternalError("Failed to create directory", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return downloads.InternalError("Failed to create file", err)
	}

	defer f.Close()

	_, err = io.Copy(f, reader)
	if err != nil {
		return downloads.InternalError("Failed to write file", err)
	}

	return nil
}
