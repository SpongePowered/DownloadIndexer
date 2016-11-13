package db

import (
	"database/sql"

	// Register PostgreSQL SQL driver
	_ "github.com/lib/pq"
)

func ConnectPostgres(url string) (*sql.DB, error) {
	return sql.Open("postgres", url)
}

func Reset(db *sql.DB) error {
	err := dropTables(db)
	if err != nil {
		return err
	}

	err = createTables(db)
	if err != nil {
		return err
	}

	return setupProjects(db)
}
