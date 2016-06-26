package maven

import (
	"bytes"
	"io"
)

type Uploader interface {
	Upload(path string, reader io.Reader) error
}

type Proxy struct {
	Target   string
	Uploader []Uploader
}

func (p *Proxy) Get(path string) string {
	return p.Target + path
}

func (p *Proxy) Upload(path string, data []byte) (err error) {
	reader := bytes.NewReader(data)

	for _, uploader := range p.Uploader {
		err = uploader.Upload(path, reader)
		if err != nil {
			return
		}

		// Reset reader
		_, err = reader.Seek(0, 0)
		if err != nil {
			return
		}
	}

	return
}
