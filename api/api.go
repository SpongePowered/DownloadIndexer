package api

import (
	"database/sql"
	"gopkg.in/macaron.v1"
	"net/http"
)

type API struct {
	db     *sql.DB
	target string
}

func Create(db *sql.DB, target string) *API {
	return &API{db, target}
}

type info struct {
	Version string `json:"version"`
}

func (a *API) GetVersion(ctx *macaron.Context) {
	ctx.JSON(http.StatusOK, info{"1.0-SNAPSHOT"})
}
