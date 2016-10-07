package main

import (
	"github.com/Minecrell/SpongeDownloads/downloads"
	"github.com/Minecrell/SpongeDownloads/downloads/api"
	"github.com/Minecrell/SpongeDownloads/downloads/db"
	"github.com/Minecrell/SpongeDownloads/downloads/git"
	"github.com/Minecrell/SpongeDownloads/downloads/indexer"
	"github.com/Minecrell/SpongeDownloads/downloads/maven"
	"github.com/go-macaron/auth"
	"gopkg.in/macaron.v1"
	"log"
	"net/http"
	"os"
	"strings"
)

func main() {
	uploadURL := requireEnv("UPLOAD_URL")
	repoURL := requireEnv("REPO_URL")

	user := requireEnv("MAVEN_USER")
	password := requireEnv("MAVEN_PASSWORD")

	postgresUrl := requireEnv("POSTGRES_URL")
	gitStorage := requireEnv("GIT_STORAGE")

	// Connect to database
	postgresDB, err := db.ConnectPostgres(postgresUrl)
	if err != nil {
		log.Fatalln(err)
	}

	// TODO
	err = db.Reset(postgresDB)
	if err != nil {
		log.Fatalln(err)
	}

	manager := &downloads.Manager{DB: postgresDB}

	// Setup upload Maven repository
	repo, err := maven.CreateRepository(uploadURL)
	if err != nil {
		log.Fatalln(err)
	}

	// Initialize Git manager
	git, err := git.Create(manager, gitStorage)
	if err != nil {
		log.Fatalln(err)
	}

	// Initialize indexer and API
	indexer := indexer.Create(manager, repo, git)
	err = indexer.LoadProjects()
	if err != nil {
		log.Fatalln(err)
	}

	api := api.Create(manager, repoURL)

	// Initialize web framework
	m := macaron.New()
	m.Use(macaron.Logger())

	m.Group("/api/v1", func() {
		m.Use(macaron.Recovery())
		m.Map(downloads.ErrorHandler(api.Log))

		m.Use(macaron.Renderer(macaron.RenderOptions{IndentJSON: true}))

		m.Get("/", api.GetProjects)
		m.Get("/*", func(ctx *macaron.Context) error {
			parts := strings.Split(ctx.Params("*"), "/")

			if len(parts) < 2 {
				ctx.Status(http.StatusNotFound)
				return nil
			}

			handler := api.GetProject
			i := len(parts) - 1

			// TODO: How can I make go-macaron recognize this directly?
			if parts[i] == "downloads" {
				i--
				handler = api.GetDownloads
			}

			return handler(ctx, maven.Identifier{strings.Join(parts[:i], "."), parts[i]})
		})
	})

	m.Group("/maven/upload", func() {
		m.Use(indexer.ErrorHandler)
		m.Use(macaron.Recovery())

		m.Map(downloads.ErrorHandler(indexer.Log))
		m.Use(auth.Basic(user, password))

		m.Get("/*", indexer.Get)
		m.Put("/*", indexer.Put)
	})

	m.Run()
}

func requireEnv(key string) string {
	value, ok := os.LookupEnv(key)
	if !ok {
		log.Fatalln(key, "is required")
	}
	return value
}
