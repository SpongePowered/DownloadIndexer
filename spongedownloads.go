package main

import (
	"github.com/Minecrell/SpongeDownloads/indexer"
	"github.com/Minecrell/SpongeDownloads/maven"
	"github.com/go-macaron/auth"
	"gopkg.in/macaron.v1"
	"log"
	"net/http"
	"os"
)

func main() {
	user := requireEnv("MAVEN_USER")
	password := requireEnv("MAVEN_PASSWORD")

	ftpHost := requireEnv("FTP_HOST")
	ftpUser := requireEnv("FTP_USER")
	ftpPassword := requireEnv("FTP_PASSWORD")

	ftpUploader, err := maven.CreateFTPUploader(ftpHost, ftpUser, ftpPassword)
	if err != nil {
		log.Fatalln(err)
	}

	target := requireEnv("MAVEN_REPO")

	// Make sure target ends with a slash
	if target[len(target)-1] != '/' {
		target += "/"
	}

	// Database
	databaseHost := requireEnv("POSTGRES_HOST")
	databaseUser := requireEnv("POSTGRES_USER")
	databasePassword := requireEnv("POSTGRES_PASSWORD")
	database := requireEnv("POSTGRES_DATABASE")

	// Initialize indexer
	indexer, err := indexer.CreatePostgres(databaseHost, databaseUser, databasePassword, database, target)
	if err != nil {
		log.Fatalln(err)
	}

	// TODO
	err = indexer.Init(true)
	if err != nil {
		log.Fatalln(err)
	}

	// Initialize Maven proxy
	proxy := &maven.Proxy{Target: target, Uploader: []maven.Uploader{indexer, ftpUploader}}

	// Initialize web framework
	m := macaron.New()
	m.Use(macaron.Logger())
	m.Use(macaron.Recovery())

	m.Group("/api/v1", func() {
		// TODO
	})

	m.Group("/maven/upload", func() {

		// Redirect to real Maven repository for metadata
		m.Get("/*", func(ctx *macaron.Context) {
			ctx.Redirect(proxy.Get(ctx.Params("*")), http.StatusMovedPermanently)
		})

		// Handle uploads to Maven repository
		m.Put("/*", func(ctx *macaron.Context) (int, string) {
			bytes, err := ctx.Req.Body().Bytes()
			if err != nil {
				panic(err) // TODO: Error handling
			}

			err = proxy.Upload(ctx.Params("*"), bytes)
			if err != nil {
				panic(err) // TODO: Error handling
			}

			return http.StatusOK, "OK"
		},
			auth.Basic(user, password)) // Use authentication for uploading

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
