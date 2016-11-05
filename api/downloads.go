package api

import (
	"database/sql"
	"encoding/json"
	"github.com/Minecrell/SpongeDownloads/db"
	"github.com/Minecrell/SpongeDownloads/downloads"
	"github.com/Minecrell/SpongeDownloads/maven"
	"github.com/lib/pq"
	"gopkg.in/macaron.v1"
	"net/http"
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

	Dependencies map[string]string    `json:"dependencies,omitempty"`
	Artifacts    map[string]*artifact `json:"artifacts"`

	Changelog json.RawMessage `json:"changelog,omitempty"`
}

type artifact struct {
	URL string `json:"url"`

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
	limit := ctx.QueryInt("limit")

	if limit <= 0 {
		limit = 10
	} else if limit > 100 {
		limit = 100
	}

	since := ctx.Query("since")
	until := ctx.Query("until")
	_, changelog := ctx.Req.Form["changelog"]

	builder := db.NewSQLBuilder("SELECT download_id, build_types.name, downloads.version, maven_version, " +
		"published, commit, label")

	if changelog {
		builder.Append(", changelog")
	}

	builder.Append(" FROM downloads JOIN build_types USING(build_type_id)")

	if dependencies != nil {
		builder.Append(" JOIN dependencies USING(download_id)")
	}

	builder.Parameter(" WHERE project_id = ", projectID)

	if buildType != "" {
		builder.Parameter(" AND build_types.name = ", buildType)
	}

	if since != "" {
		builder.Parameter(" AND published > ", since)
	}

	if until != "" {
		builder.Parameter(" AND published < ", until)
	}

	for _, dep := range dependencies {
		builder.Parameter(" AND dependencies.name = ", dep[0])
		builder.Parameter(" AND dependencies.version = ", dep[1])
	}

	builder.Parameter(" ORDER BY published DESC LIMIT ", limit)
	builder.End()

	rows, err = a.DB.Query(builder.String(), builder.Args()...)
	if err != nil {
		return downloads.InternalError("Database error (failed to lookup downloads)", err)
	}

	var downloadIDs []int64
	var downloadsSlice []*download
	downloadsMap := make(map[int]*download)

	for rows.Next() {
		var id int
		dl := &download{Dependencies: make(map[string]string), Artifacts: make(map[string]*artifact)}
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
		artifact := new(artifact)
		var classifier, extension string

		err = rows.Scan(&downloadID, &classifier, &extension, &artifact.Size, &artifact.SHA1, &artifact.MD5)
		if err != nil {
			return downloads.InternalError("Database error (failed to read artifacts)", err)
		}

		dl := downloadsMap[downloadID]

		artifact.URL = urlPrefix + defaultWhenNil(dl.mavenVersion, dl.Version) + "/" +
			project.ArtifactID + "-" + dl.Version

		if classifier != "" {
			artifact.URL += "-" + classifier
		}

		artifact.URL += "." + extension
		dl.Artifacts[classifier] = artifact
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
