package downloads

import (
	"database/sql"
	"github.com/SpongePowered/DownloadIndexer/cache"
	"gopkg.in/macaron.v1"
	"log"
	"os"
)

type Manager struct {
	DB    *sql.DB
	Cache cache.Cache
}

func CreateLogger(name string) *log.Logger {
	return log.New(os.Stdout, "["+name+"] ", log.LstdFlags)
}

func (m *Manager) Module(name string) *Module {
	return &Module{m, CreateLogger(name)}
}

type Module struct {
	*Manager
	Log *log.Logger
}

func (m *Module) InitializeContext(ctx *macaron.Context) {
	ctx.Map(m.Log)
}
