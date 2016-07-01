package api

import (
	"bytes"
	"database/sql"
	"gopkg.in/macaron.v1"
	"net/http"
	"time"
)

type download struct {
	id uint

	Version         string    `json:"version"`
	SnapshotVersion string    `json:"snapshotVersion,omitempty"`
	Type            string    `json:"type"`
	Minecraft       string    `json:"minecraft"`
	Commit          string    `json:"commit"`
	Label           string    `json:"label,omitempty"`
	Published       time.Time `json:"published"`
}

func (a *API) GetDownloads(ctx *macaron.Context) {
	identifier := ctx.Params("project")

	var projectID uint

	row := a.db.QueryRow("SELECT id FROM projects WHERE identifier = $1;", identifier)
	err := row.Scan(&projectID)
	if err != nil {
		panic(err)
	}

	buildType := ctx.Query("type")
	minecraft := ctx.Query("minecraft")
	limit := ctx.QueryInt("limit")

	if limit > 100 || limit <= 0 {
		limit = 100
	}

	until := ctx.Query("until")
	changelog := ctx.Query("changelog") != ""

	args := make([]interface{}, 1, 5)
	args[0] = projectID

	buffer := bytes.NewBufferString("SELECT downloads.id, downloads.version, downloads.snapshot_version, branches.type, downloads.minecraft, " +
		"downloads.commit, downloads.label, downloads.published " +
		"FROM downloads LEFT OUTER JOIN branches ON (downloads.branch_id = branches.id) " +
		"WHERE downloads.project_id = $1")

	var i byte
	i = 2

	if buildType != "" {
		args = append(args, buildType)
		buffer.WriteString(" AND branches.type = $")
		buffer.WriteByte('0' + i)
		i++
	}

	if minecraft != "" {
		args = append(args, minecraft)
		buffer.WriteString(" AND minecraft = $")
		buffer.WriteByte('0' + i)
		i++
	}

	if until != "" {
		args = append(args, until)
		buffer.WriteString(" AND published < $4 ")
		buffer.WriteByte('0' + i)
		i++
	}

	args = append(args, limit)
	buffer.WriteString(" ORDER BY downloads.published DESC LIMIT $")
	buffer.WriteByte('0' + i)
	buffer.WriteByte(';')

	rows, err := a.db.Query(buffer.String(), args...)
	if err != nil {
		panic(err)
	}

	var downloads []download

	for rows.Next() {
		var dl download
		var label sql.NullString

		err = rows.Scan(&dl.id, &dl.Version, &dl.SnapshotVersion, &dl.Type, &dl.Minecraft, &dl.Commit, &label, &dl.Published)
		if err != nil {
			panic(err)
		}

		dl.Label = label.String
		downloads = append(downloads, dl)
	}

	rows.Close()

	if changelog {

	}

	ctx.JSON(http.StatusOK, downloads)
}
