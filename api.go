package main

import (
	"github.com/Minecrell/SpongeDownloads/downloads"
	"github.com/Minecrell/SpongeDownloads/downloads/api"
	"gopkg.in/macaron.v1"
)

func setupAPI(manager *downloads.Manager, m *macaron.Macaron) {
	repoURL := requireEnv("REPO_URL")
	api.Create(manager, repoURL).Setup(m)
}
