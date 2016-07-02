package api

import "github.com/Minecrell/SpongeDownloads/downloads"

type API struct {
	*downloads.Service
}

func Create(m *downloads.Manager) *API {
	return &API{m.Service("API")}
}
