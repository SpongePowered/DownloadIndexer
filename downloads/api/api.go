package api

import (
	"github.com/Minecrell/SpongeDownloads/downloads"
	"gopkg.in/macaron.v1"
)

type API struct {
	*downloads.Service
	Repo string
}

func Create(m *downloads.Manager, repo string) *API {
	// Make sure repo URL ends with a slash
	if repo[len(repo)-1] != '/' {
		repo += "/"
	}

	return &API{m.Service("API"), repo}
}

func (a *API) AddHeaders(ctx *macaron.Context) {
	header := ctx.Header()
	header.Add("Access-Control-Allow-Origin", "*") // TODO
}
