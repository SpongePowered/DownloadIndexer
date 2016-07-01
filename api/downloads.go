package api

import (
	"bytes"
	"database/sql"
	"gopkg.in/macaron.v1"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type download struct {
	Version         string    `json:"version"`
	SnapshotVersion *string   `json:"snapshotVersion,omitempty"`
	Type            string    `json:"type"`
	Minecraft       string    `json:"minecraft"`
	Commit          string    `json:"commit"`
	Label           string    `json:"label,omitempty"`
	Published       time.Time `json:"published"`

	Artifacts []*artifact `json:"artifacts"`
}

type artifact struct {
	Classifier *string `json:"classifier,omitempty"`
	Extension  string  `json:"extension"`
	URL        string  `json:"url"`
	SHA1       string  `json:"sha1,omitempty"`
	MD5        string  `json:"md5,omitempty"`
}

func (a *API) GetDownloads(ctx *macaron.Context) {
	identifier := ctx.Params("project")

	var projectID uint
	var groupID, artifactID string

	row := a.db.QueryRow("SELECT id, group_id, artifact_id FROM projects WHERE identifier = $1;", identifier)
	err := row.Scan(&projectID, &groupID, &artifactID)
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

	var ids bytes.Buffer
	ids.WriteByte('(')

	first := true

	var downloads []*download
	downloadsMap := make(map[int]*download)

	for rows.Next() {
		var id int
		var dl download
		var label sql.NullString

		err = rows.Scan(&id, &dl.Version, &dl.SnapshotVersion, &dl.Type, &dl.Minecraft, &dl.Commit, &label, &dl.Published)
		if err != nil {
			panic(err)
		}

		dl.Label = label.String

		if first {
			first = false
		} else {
			ids.WriteByte(',')
		}

		ids.WriteString(strconv.Itoa(id))

		downloads = append(downloads, &dl)
		downloadsMap[id] = &dl
	}

	ids.WriteByte(')')

	rows.Close()

	rows, err = a.db.Query("SELECT download_id, classifier, extension, sha1, md5 FROM artifacts WHERE download_id IN " + ids.String() + ";")
	if err != nil {
		panic(err)
	}

	urlPrefix := a.target + strings.Replace(groupID, ".", "/", -1) + "/" + artifactID + "/"

	for rows.Next() {
		var id int
		var artifact artifact

		err = rows.Scan(&id, &artifact.Classifier, &artifact.Extension, &artifact.SHA1, &artifact.MD5)
		if err != nil {
			panic(err)
		}

		dl := downloadsMap[id]

		artifact.URL = urlPrefix + dl.Version + "/" + artifactID + "-" + nilFallback(dl.SnapshotVersion, dl.Version)

		if artifact.Classifier != nil {
			artifact.URL += "-" + *artifact.Classifier
		}

		artifact.URL += "." + artifact.Extension

		dl.Artifacts = append(dl.Artifacts, &artifact)
	}

	if changelog {

	}

	ctx.JSON(http.StatusOK, downloads)
}

func nilFallback(a *string, b string) string {
	if a == nil {
		return b
	}
	return *a
}
