package main

import (
	"github.com/SpongePowered/SpongeDownloads/downloads"
	"github.com/SpongePowered/SpongeDownloads/git"
	"github.com/SpongePowered/SpongeDownloads/indexer"
	"github.com/SpongePowered/SpongeDownloads/maven"
	"gopkg.in/macaron.v1"
)

func setupIndexer(manager *downloads.Manager, m *macaron.Macaron) {
	authHandler := setupAuthentication("UPLOAD_AUTH")
	uploadURL := requireEnv("UPLOAD_URL")
	gitStorage := requireEnv("GIT_STORAGE_DIR")

	// Setup upload Maven repository
	repo, err := maven.CreateRepository(uploadURL)
	if err != nil {
		logger.Fatalln(err)
	}

	// Initialize Git manager
	gitManager, err := git.Create(manager, gitStorage)
	if err != nil {
		logger.Fatalln(err)
	}

	// Initialize indexer and API
	i := indexer.Create(manager, repo, gitManager)
	err = i.LoadProjects()
	if err != nil {
		logger.Fatalln(err)
	}

	i.Setup(m, authHandler)
}
