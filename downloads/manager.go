package downloads

import (
	"database/sql"
	"github.com/Minecrell/SpongeDownloads/downloads/repo"
	"log"
	"os"
)

type Manager struct {
	Repo string
	DB   *sql.DB

	Git *repo.Manager
}

func (m *Manager) CreateLogger(name string) *log.Logger {
	return log.New(os.Stdout, "["+name+"] ", log.LstdFlags)
}

func (m *Manager) Service(name string) *Service {
	return &Service{m, m.CreateLogger(name)}
}

type Service struct {
	*Manager
	Log *log.Logger
}
