package db

import "database/sql"

func ToNullString(s string) (result sql.NullString) {
	if s != "" {
		result.String = s
		result.Valid = true
	}

	return
}
