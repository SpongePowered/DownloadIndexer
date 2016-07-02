package downloads

import (
	"database/sql"
	"log"
	"os"
)

type Manager struct {
	Repo string
	DB   *sql.DB
}

func (m *Manager) Service(name string) *Service {
	return &Service{m.Repo, m.DB, log.New(os.Stdout, "["+name+"] ", log.LstdFlags)}
}

type Service struct {
	Repo string
	DB   *sql.DB
	Log  *log.Logger
}
