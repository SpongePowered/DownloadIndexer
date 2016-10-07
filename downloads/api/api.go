package api

import "github.com/Minecrell/SpongeDownloads/downloads"

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
