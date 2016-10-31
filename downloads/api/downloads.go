package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"github.com/Minecrell/SpongeDownloads/downloads"
	"github.com/Minecrell/SpongeDownloads/downloads/maven"
	"github.com/lib/pq"
	"gopkg.in/macaron.v1"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var emptyArray [0]struct{}

type download struct {
	Version      string `json:"version"`
	mavenVersion *string
	Published    time.Time `json:"published"`
	Type         string    `json:"type"`
	Commit       string    `json:"commit"`
	Label        *string   `json:"label,omitempty"`

	Dependencies map[string]string `json:"dependencies,omitempty"`
	Artifacts    []*artifact       `json:"artifacts"`

	Changelog json.RawMessage `json:"changelog,omitempty"`
}

type artifact struct {
	URL  string `json:"url"`
	Size int    `json:"size"`
	SHA1 string `json:"sha1"`
	MD5  string `json:"md5"`
}

func (a *API) GetDownloads(ctx *macaron.Context, project maven.Identifier) error {
	var projectID int

	// Lookup project
	err := a.DB.QueryRow("SELECT project_id FROM projects WHERE group_id = $1 AND artifact_id = $2;",
		project.GroupID, project.ArtifactID).Scan(&projectID)
	if err != nil {
		if err == sql.ErrNoRows {
			return downloads.NotFound("Unknown project")
		}
		return downloads.InternalError("Database error (failed to lookup project)", err)
	}

	// Get possible dependencies
	rows, err := a.DB.Query("SELECT DISTINCT name FROM dependencies "+
		"JOIN downloads USING (download_id)"+
		"WHERE project_id = $1;", projectID)
	if err != nil {
		return downloads.InternalError("Database error (failed to lookup dependencies)", err)
	}

	var dependencies [][2]string
	for rows.Next() {
		var name string
		rows.Scan(&name)

		if val := ctx.Query(name); val != "" {
			dependencies = append(dependencies, [2]string{name, val})
		}
	}

	buildType := ctx.Query("type")
	minecraft := ctx.Query("minecraft")
	limit := ctx.QueryInt("limit")

	if limit <= 0 {
		limit = 10
	} else if limit > 100 {
		limit = 100
	}

	since := ctx.Query("since")
	until := ctx.Query("until")
	_, changelog := ctx.Req.Form["changelog"]

	args := make([]interface{}, 1, 10)
	args[0] = projectID

	buffer := bytes.NewBufferString("SELECT download_id, build_types.name, downloads.version, maven_version, " +
		"published, commit, label")

	if changelog {
		buffer.WriteString(", changelog")
	}

	buffer.WriteString(" FROM downloads JOIN build_types USING(build_type_id)")

	if dependencies != nil {
		buffer.WriteString(" JOIN dependencies USING(download_id)")
	}

	buffer.WriteString(" WHERE project_id = $1")

	var i int
	i = 2

	if buildType != "" {
		args = append(args, buildType)
		buffer.WriteString(" AND build_types.name = $")
		buffer.WriteString(strconv.Itoa(i))
		i++
	}

	if minecraft != "" {
		args = append(args, minecraft)
		buffer.WriteString(" AND minecraft = $")
		buffer.WriteString(strconv.Itoa(i))
		i++
	}

	if since != "" {
		args = append(args, since)
		buffer.WriteString(" AND published > $")
		buffer.WriteString(strconv.Itoa(i))
		i++
	}

	if until != "" {
		args = append(args, until)
		buffer.WriteString(" AND published < $")
		buffer.WriteString(strconv.Itoa(i))
		i++
	}

	for _, dep := range dependencies {
		args = append(args, dep[0], dep[1])
		buffer.WriteString(" AND name = $")
		buffer.WriteString(strconv.Itoa(i))
		i++
		buffer.WriteString(" AND dependencies.version = $")
		buffer.WriteString(strconv.Itoa(i))
		i++
	}

	args = append(args, limit)
	buffer.WriteString(" ORDER BY published DESC LIMIT $")
	buffer.WriteString(strconv.Itoa(i))
	buffer.WriteByte(';')

	rows, err = a.DB.Query(buffer.String(), args...)
	if err != nil {
		return downloads.InternalError("Database error (failed to lookup downloads)", err)
	}

	var downloadIDs []int64
	var downloadsSlice []*download
	downloadsMap := make(map[int]*download)

	for rows.Next() {
		var id int
		dl := &download{Dependencies: make(map[string]string)}
		var changelogJSON []byte

		if changelog {
			err = rows.Scan(&id, &dl.Type, &dl.Version, &dl.mavenVersion, &dl.Published, &dl.Commit, &dl.Label,
				&changelogJSON)
		} else {
			err = rows.Scan(&id, &dl.Type, &dl.Version, &dl.mavenVersion, &dl.Published, &dl.Commit, &dl.Label)
		}

		if err != nil {
			return downloads.InternalError("Database error (failed to read downloads)", err)
		}

		dl.Changelog = json.RawMessage(changelogJSON)

		downloadIDs = append(downloadIDs, int64(id))
		downloadsSlice = append(downloadsSlice, dl)
		downloadsMap[id] = dl
	}

	if downloadsSlice == nil {
		// No downloads available
		ctx.JSON(http.StatusOK, emptyArray)
		return nil
	}

	// Get download dependencies
	rows, err = a.DB.Query("SELECT download_id, name, version FROM dependencies "+
		"WHERE download_id = ANY($1);", pq.Array(downloadIDs))
	if err != nil {
		return downloads.InternalError("Database error (failed to lookup dependencies)", err)
	}

	for rows.Next() {
		var downloadID int
		var name, version string
		err = rows.Scan(&downloadID, &name, &version)
		if err != nil {
			return downloads.InternalError("Database error (failed to read dependencies)", err)
		}

		downloadsMap[downloadID].Dependencies[name] = version
	}

	// Get download artifacts
	rows, err = a.DB.Query("SELECT download_id, classifier, extension, size, sha1, md5 FROM artifacts "+
		"WHERE download_id = ANY($1);", pq.Array(downloadIDs))
	if err != nil {
		return downloads.InternalError("Database error (failed to lookup artifacts)", err)
	}

	urlPrefix := a.Repo + strings.Replace(project.GroupID, ".", "/", -1) + "/" + project.ArtifactID + "/"

	for rows.Next() {
		var downloadID int
		var artifact artifact
		var classifier, extension string

		err = rows.Scan(&downloadID, &classifier, &extension, &artifact.Size, &artifact.SHA1, &artifact.MD5)
		if err != nil {
			return downloads.InternalError("Database error (failed to read artifacts)", err)
		}

		dl := downloadsMap[downloadID]

		artifact.URL = urlPrefix + dl.Version + "/" + project.ArtifactID + "-" + defaultWhenNil(dl.mavenVersion, dl.Version)

		if classifier != "" {
			artifact.URL += "-" + classifier
		}

		artifact.URL += "." + extension

		dl.Artifacts = append(dl.Artifacts, &artifact)
	}

	ctx.JSON(http.StatusOK, downloadsSlice)
	return nil
}

func defaultWhenNil(a *string, b string) string {
	if a == nil {
		return b
	}
	return *a
}
