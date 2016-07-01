package db

import "database/sql"

func ReadRows(rows *sql.Rows) (result []string) {
	defer rows.Close()

	var row string

	for rows.Next() {
		rows.Scan(&row)
		result = append(result, row)
	}

	return
}

func ToNullString(s string) (result sql.NullString) {
	if s != "" {
		result.String = s
		result.Valid = true
	}

	return
}
