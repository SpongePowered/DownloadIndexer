package api

import "github.com/Minecrell/SpongeDownloads/downloads"

type API struct {
	*downloads.Service
	Repo string
}

func Create(m *downloads.Manager, repo string) *API {
	return &API{m.Service("API"), repo}
}
