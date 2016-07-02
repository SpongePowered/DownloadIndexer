package maven

import (
	"gopkg.in/macaron.v1"
	"net/http"
)

type Uploader interface {
	Upload(path string, data []byte) error
}

type Proxy struct {
	Repo     string
	Uploader []Uploader
}

func (p *Proxy) Redirect(ctx *macaron.Context) {
	ctx.Redirect(p.Repo+ctx.Params("*"), http.StatusMovedPermanently)
}

func (p *Proxy) Upload(ctx *macaron.Context) (err error) {
	path := ctx.Params("*")
	data, err := ctx.Req.Body().Bytes()
	if err != nil {
		return
	}

	for _, uploader := range p.Uploader {
		err = uploader.Upload(path, data)
		if err != nil {
			return
		}
	}

	return
}
