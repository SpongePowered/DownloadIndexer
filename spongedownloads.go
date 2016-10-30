package main

import (
	"database/sql"
	"github.com/Minecrell/SpongeDownloads/downloads"
	"github.com/Minecrell/SpongeDownloads/downloads/auth"
	"github.com/Minecrell/SpongeDownloads/downloads/db"
	"gopkg.in/macaron.v1"
	"log"
	"os"
	"strings"
)

func main() {
	// Parse module configuration
	enableIndexer, enableAPI, enablePromote := true, true, true

	if modules := parseModules("MODULES"); modules != nil {
		enableIndexer = modules.isEnabled("indexer")
		enableAPI = modules.isEnabled("api")
		enablePromote = modules.isEnabled("promote")
	}

	// Setup database and create manager
	manager := &downloads.Manager{DB: setupDatabase()}

	// Initialize web framework
	m := macaron.New()
	m.Use(macaron.Logger())
	m.Map(downloads.ErrorHandler())

	authHandler := setupAuthentication("API_AUTH")

	if enableIndexer {
		setupIndexer(manager, m, authHandler)
	}

	if enableAPI {
		setupAPI(manager, m)
	}

	if enablePromote {
		// TODO
	}

	m.Run()
}

func setupDatabase() *sql.DB {
	postgresDB, err := db.ConnectPostgres(requireEnv("POSTGRES_URL"))
	if err != nil {
		log.Fatalln(err)
	}

	// TODO
	/*err = db.Reset(postgresDB)
	if err != nil {
		log.Fatalln(err)
	}*/

	return postgresDB
}

func setupAuthentication(key string) macaron.Handler {
	value := requireEnv(key)
	if value != "" {
		return auth.Basic([]byte(value))
	} else {
		return func() {}
	}
}

func requireEnv(key string) string {
	value, ok := os.LookupEnv(key)
	if !ok {
		log.Fatalln(key, "is required")
	}
	return value
}

type modules []string

func parseModules(key string) modules {
	components, ok := os.LookupEnv(key)
	if ok {
		m := strings.Split(components, ",")
		if len(m) == 0 {
			log.Fatalln("Cannot disable all modules")
		}

		return m
	} else {
		return nil
	}
}

func (m modules) isEnabled(name string) bool {
	for _, v := range m {
		if v == name {
			return true
		}
	}
	return false
}
