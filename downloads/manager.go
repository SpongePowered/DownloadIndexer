package downloads

import (
	"database/sql"
	"gopkg.in/macaron.v1"
	"log"
	"os"
)

type Manager struct {
	DB *sql.DB
}

func createLogger(name string) *log.Logger {
	return log.New(os.Stdout, "["+name+"] ", log.LstdFlags)
}

func (m *Manager) Module(name string) *Module {
	return &Module{m, createLogger(name)}
}

type Module struct {
	*Manager
	Log *log.Logger
}

func (m *Module) InitializeContext(ctx *macaron.Context) {
	ctx.Map(m.Log)
}
