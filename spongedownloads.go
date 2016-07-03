package main

import (
	"github.com/Minecrell/SpongeDownloads/downloads"
	"github.com/Minecrell/SpongeDownloads/downloads/api"
	"github.com/Minecrell/SpongeDownloads/downloads/db"
	"github.com/Minecrell/SpongeDownloads/downloads/indexer"
	"github.com/Minecrell/SpongeDownloads/downloads/maven"
	"github.com/Minecrell/SpongeDownloads/downloads/repo"
	"github.com/go-macaron/auth"
	"gopkg.in/macaron.v1"
	"log"
	"net/http"
	"os"
	"strings"
)

func main() {
	mavenRepo := requireEnv("MAVEN_REPO")
	user := requireEnv("MAVEN_USER")
	password := requireEnv("MAVEN_PASSWORD")

	ftpHost := requireEnv("FTP_HOST")
	ftpUser := requireEnv("FTP_USER")
	ftpPassword := requireEnv("FTP_PASSWORD")

	// Make sure target ends with a slash
	if mavenRepo[len(mavenRepo)-1] != '/' {
		mavenRepo += "/"
	}

	postgresUrl := requireEnv("POSTGRES_URL")
	repoStorage := requireEnv("REPO_STORAGE")

	// Connect to database
	postgresDB, err := db.ConnectPostgres(postgresUrl)
	if err != nil {
		log.Fatalln(err)
	}

	// TODO
	/*err = db.Reset(postgresDB)
	if err != nil {
		log.Fatalln(err)
	}*/

	manager := &downloads.Manager{Repo: mavenRepo, DB: postgresDB}

	// Initialize repo manager
	repo, err := repo.Create(manager.CreateLogger("Git"), repoStorage)
	if err != nil {
		log.Fatalln(err)
	}

	manager.Git = repo

	// Setup FTP uploader (does not connect to the FTP server yet)
	ftpUploader, err := maven.CreateFTPUploader(ftpHost, ftpUser, ftpPassword)
	if err != nil {
		log.Fatalln(err)
	}

	// Initialize indexer and API
	indexer := indexer.Create(manager)
	api := api.Create(manager)

	// Initialize Maven proxy
	proxy := &maven.Proxy{Repo: mavenRepo, Uploader: []maven.Uploader{indexer, ftpUploader}}

	// Initialize web framework
	m := macaron.New()
	m.Use(macaron.Logger())
	m.Use(macaron.Recovery())

	m.Group("/api/v1", func() {

		m.Map(downloads.ErrorHandler(api.Log))

		m.Use(macaron.Renderer())

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

			return handler(ctx, maven.Coordinates{strings.Join(parts[:i], "."), parts[i]})
		})
	})

	m.Group("/maven/upload", func() {
		m.Map(downloads.ErrorHandler(indexer.Log))

		m.Get("/*", proxy.Redirect)
		m.Put("/*", auth.Basic(user, password), proxy.Upload)
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
