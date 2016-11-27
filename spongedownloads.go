package main // import "github.com/SpongePowered/SpongeDownloads"

import (
	"database/sql"
	"github.com/SpongePowered/SpongeDownloads/auth"
	"github.com/SpongePowered/SpongeDownloads/cache"
	"github.com/SpongePowered/SpongeDownloads/db"
	"github.com/SpongePowered/SpongeDownloads/downloads"
	"github.com/SpongePowered/SpongeDownloads/httperror"
	"github.com/SpongePowered/SpongeWebGo"
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
	if cacheConfig := os.Getenv("CACHE"); cacheConfig != "" {
		var err error
		c, err = cache.Create(downloads.CreateLogger("Cache"), cacheConfig)
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
	m.Map(httperror.Handler())

	// Setup logging handler
	if c != nil {
		m.Use(c.LogHandler())
	} else {
		m.Use(macaron.Logger())
	}

	m.Use(swg.AddHeaders)
	m.Use(addHeaders)

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

func addHeaders(resp http.ResponseWriter) {
	resp.Header().Add("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
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
