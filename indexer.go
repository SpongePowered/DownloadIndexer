package main

import (
	"github.com/Minecrell/SpongeDownloads/downloads"
	"github.com/Minecrell/SpongeDownloads/downloads/git"
	"github.com/Minecrell/SpongeDownloads/downloads/indexer"
	"github.com/Minecrell/SpongeDownloads/downloads/maven"
	"gopkg.in/macaron.v1"
	"log"
)

func setupIndexer(manager *downloads.Manager, m *macaron.Macaron, auth macaron.Handler) {
	uploadURL := requireEnv("UPLOAD_URL")
	gitStorage := requireEnv("GIT_STORAGE_DIR")

	// Setup upload Maven repository
	repo, err := maven.CreateRepository(uploadURL)
	if err != nil {
		log.Fatalln(err)
	}

	// Initialize Git manager
	gitManager, err := git.Create(manager, gitStorage)
	if err != nil {
		log.Fatalln(err)
	}

	// Initialize indexer and API
	i := indexer.Create(manager, repo, gitManager)
	err = i.LoadProjects()
	if err != nil {
		log.Fatalln(err)
	}

	i.Setup(m, auth)
}
