package db

import (
	"bytes"
	"database/sql"
	"strconv"
)

func ToNullString(s string) (result sql.NullString) {
	if s != "" {
		result.String = s
		result.Valid = true
	}

	return
}

type SQLBuilder struct {
	buffer *bytes.Buffer

	i    int
	args []interface{}
}

func NewSQLBuilder(sql string) *SQLBuilder {
	return &SQLBuilder{buffer: bytes.NewBufferString(sql)}
}

func (b *SQLBuilder) Append(sql string) {
	b.buffer.WriteString(sql)
}

func (b *SQLBuilder) Parameter(sql string, val interface{}) {
	b.i++
	b.Append(sql)
	b.buffer.WriteByte('$')
	b.Append(strconv.Itoa(b.i))
	b.args = append(b.args, val)
}

func (b *SQLBuilder) End() {
	b.buffer.WriteByte(';')
}

func (b *SQLBuilder) String() string {
	return b.buffer.String()
}

func (b *SQLBuilder) Args() []interface{} {
	return b.args
}
