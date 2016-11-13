package main

import (
	"database/sql"
	"github.com/Minecrell/SpongeDownloads/auth"
	"github.com/Minecrell/SpongeDownloads/cache"
	"github.com/Minecrell/SpongeDownloads/db"
	"github.com/Minecrell/SpongeDownloads/downloads"
	"github.com/Minecrell/SpongeDownloads/httperror"
	"gopkg.in/macaron.v1"
	"net/http"
	"os"
	"strings"
)

var logger = downloads.CreateLogger("Main")

func main() {
	// Parse module configuration
	enableIndexer, enableAPI, enablePromote := true, true, true

	if modules := parseModules("MODULES"); modules != nil {
		logger.Println("Enabled modules:", modules)
		enableIndexer = modules.isEnabled("indexer")
		enableAPI = modules.isEnabled("api")
		enablePromote = modules.isEnabled("promote")
	}

	var c cache.Cache
	if cacheConfig := os.Getenv("CACHE_PROXY"); cacheConfig != "" {
		var err error
		c, err = cache.Create(cacheConfig)
		if err != nil {
			logger.Fatalln(err)
		}
	}

	// Setup database and create manager
	manager := &downloads.Manager{
		DB:    setupDatabase(),
		Cache: c,
	}

	// Initialize web framework
	m := macaron.New()
	m.Use(macaron.Logger())
	m.Map(httperror.Handler())

	renderer := macaron.Renderer(macaron.RenderOptions{IndentJSON: macaron.Env == macaron.DEV})

	if redirectRoot := os.Getenv("REDIRECT_ROOT"); redirectRoot != "" {
		m.Get("/", func(ctx *macaron.Context) {
			ctx.Redirect(redirectRoot, http.StatusMovedPermanently)
		})
	}

	if statusz := statuszHandler(); statusz != nil {
		m.Get("/statusz", renderer, statusz)
	}

	if enableIndexer {
		logger.Println("Starting indexer")
		setupIndexer(manager, m)
	}

	if enableAPI {
		logger.Println("Starting API")
		setupAPI(manager, m, renderer)
	}

	if enablePromote {
		// TODO
	}

	m.Run()
}

func setupDatabase() *sql.DB {
	logger.Println("Connecting to database")

	postgresDB, err := db.ConnectPostgres(requireEnv("POSTGRES_URL"))
	if err != nil {
		logger.Fatalln(err)
	}

	// TODO
	/*err = db.Reset(postgresDB)
	if err != nil {
		logger.Fatalln(err)
	}*/

	return postgresDB
}

func setupAuthentication(key string) macaron.Handler {
	value := requireEnv(key)
	if value != "" {
		return auth.Basic([]byte(value))
	}

	return func() {}
}

func requireEnv(key string) string {
	value, ok := os.LookupEnv(key)
	if !ok {
		logger.Fatalln(key, "is required")
	}
	return value
}

type modules []string

func parseModules(key string) modules {
	components, ok := os.LookupEnv(key)
	if ok {
		m := strings.Split(components, ",")
		if len(m) == 0 {
			logger.Fatalln("Cannot disable all modules")
		}

		return m
	}

	return nil
}

func (m modules) isEnabled(name string) bool {
	for _, v := range m {
		if v == name {
			return true
		}
	}
	return false
}
