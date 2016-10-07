package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"github.com/Minecrell/SpongeDownloads/downloads"
	"github.com/Minecrell/SpongeDownloads/downloads/maven"
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
	parentCommit    string

	Artifacts []*artifact     `json:"artifacts"`
	Changelog json.RawMessage `json:"changelog,omitempty"`
}

type artifact struct {
	Classifier string `json:"classifier,omitempty"`
	Extension  string `json:"extension"`
	URL        string `json:"url"`
	Size       int    `json:"size"`
	SHA1       string `json:"sha1,omitempty"`
	MD5        string `json:"md5,omitempty"`
}

func (a *API) GetDownloads(ctx *macaron.Context, project maven.Identifier) error {
	var projectID uint

	row := a.DB.QueryRow("SELECT id FROM projects WHERE group_id = $1 AND artifact_id = $2;", project.GroupID, project.ArtifactID)
	err := row.Scan(&projectID)
	if err != nil {
		return downloads.InternalError("Database error (failed to lookup project)", err)
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
		"downloads.commit, downloads.label, downloads.published")

	if changelog {
		buffer.WriteString(", downloads.changelog")
	}

	buffer.WriteString(" FROM downloads LEFT OUTER JOIN branches ON (downloads.branch_id = branches.id) " +
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

	rows, err := a.DB.Query(buffer.String(), args...)
	if err != nil {
		return downloads.InternalError("Database error (failed to lookup downloads)", err)
	}

	var ids bytes.Buffer
	ids.WriteByte('(')

	first := true

	var downloadsSlice []*download
	downloadsMap := make(map[int]*download)

	for rows.Next() {
		var id int
		var dl download
		var label sql.NullString
		var changelogJSON []byte

		if changelog {
			err = rows.Scan(&id, &dl.Version, &dl.SnapshotVersion, &dl.Type, &dl.Minecraft, &dl.Commit, &label,
				&dl.Published, &changelogJSON)
		} else {
			err = rows.Scan(&id, &dl.Version, &dl.SnapshotVersion, &dl.Type, &dl.Minecraft, &dl.Commit, &label,
				&dl.Published)
		}

		if err != nil {
			return downloads.InternalError("Database error (failed to read downloads)", err)
		}

		dl.Label = label.String
		dl.Changelog = json.RawMessage(changelogJSON)

		if first {
			first = false
		} else {
			ids.WriteByte(',')
		}

		ids.WriteString(strconv.Itoa(id))

		downloadsSlice = append(downloadsSlice, &dl)
		downloadsMap[id] = &dl
	}

	ids.WriteByte(')')

	rows.Close()

	rows, err = a.DB.Query("SELECT download_id, classifier, extension, size, sha1, md5 FROM artifacts WHERE download_id IN " + ids.String() + ";")
	if err != nil {
		return downloads.InternalError("Database error (failed to lookup artifacts)", err)
	}

	urlPrefix := a.Repo + strings.Replace(project.GroupID, ".", "/", -1) + "/" + project.ArtifactID + "/"

	for rows.Next() {
		var id int
		var artifact artifact

		err = rows.Scan(&id, &artifact.Classifier, &artifact.Extension, &artifact.Size, &artifact.SHA1, &artifact.MD5)
		if err != nil {
			return downloads.InternalError("Database error (failed to read artifacts)", err)
		}

		dl := downloadsMap[id]

		artifact.URL = urlPrefix + dl.Version + "/" + project.ArtifactID + "-" + nilFallback(dl.SnapshotVersion, dl.Version)

		if artifact.Classifier != "" {
			artifact.URL += "-" + artifact.Classifier
		}

		artifact.URL += "." + artifact.Extension

		dl.Artifacts = append(dl.Artifacts, &artifact)
	}

	ctx.JSON(http.StatusOK, downloadsSlice)
	return nil
}

func nilFallback(a *string, b string) string {
	if a == nil {
		return b
	}
	return *a
}
