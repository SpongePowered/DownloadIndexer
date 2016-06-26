package main

import (
	"github.com/Minecrell/SpongeDownloads/indexer"
	"github.com/Minecrell/SpongeDownloads/maven"
	"gopkg.in/macaron.v1"
	"log"
	"net/http"
	"os"
)

func main() {
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

	// Initialize indexer
	indexer := &indexer.Indexer{Target: target}

	// Initialize Maven proxy
	proxy := &maven.Proxy{Target: target, Uploader: []maven.Uploader{indexer, ftpUploader}}

	// Initialize web framework
	m := macaron.New()
	m.Use(macaron.Logger())
	m.Use(macaron.Recovery())

	m.Group("/api", func() {
		m.Group("/v1", func() {
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
			})

		})
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
