package main

import (
	"github.com/SpongePowered/DownloadIndexer/api"
	"github.com/SpongePowered/DownloadIndexer/downloads"
	"gopkg.in/macaron.v1"
)

func setupAPI(manager *downloads.Manager, m *macaron.Macaron, renderer macaron.Handler) {
	repoURL := requireEnv("REPO_URL")
	api.Create(manager, repoURL).Setup(m, renderer)
}
