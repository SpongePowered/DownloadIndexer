package api

import (
	"database/sql"
	"encoding/json"
	"github.com/SpongePowered/DownloadIndexer/db"
	"github.com/SpongePowered/DownloadIndexer/httperror"
	"github.com/SpongePowered/DownloadIndexer/maven"
	"github.com/lib/pq"
	"gopkg.in/macaron.v1"
	"net/http"
	"strings"
	"time"
)

const recommendedLabel = "recommended"

var (
	emptyArray [0]struct{}
)

type download struct {
	Version         string `json:"version"`
	snapshotVersion *string
	Published       time.Time `json:"published"`
	Type            string    `json:"type"`
	Commit          string    `json:"commit"`
	Label           *string   `json:"label,omitempty"`

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

func (a *API) GetDownload(ctx *macaron.Context, project maven.Identifier) error {
	q, err := a.createDownloadQuery(ctx, project)
	if err != nil {
		return err
	}

	q.init(false)
	q.builder.Parameter("AND version = ", ctx.Params("version"))

	dls, err := q.Read(a, project)
	if err != nil {
		return err
	}

	if dls == nil {
		return httperror.NotFound("Unknown version")
	}

	ctx.JSON(http.StatusOK, dls[0])
	return nil
}

func (a *API) GetRecommendedDownload(ctx *macaron.Context, project maven.Identifier) error {
	dls, err := a.filterDownloads(ctx, project, false, recommendedLabel)
	if err != nil {
		return err
	}

	if dls == nil {
		return httperror.NotFound("No recommended version found")
	}

	ctx.JSON(http.StatusOK, dls[0])
	return nil
}

func (a *API) GetDownloads(ctx *macaron.Context, project maven.Identifier) error {
	dls, err := a.filterDownloads(ctx, project, true, "")
	if err != nil {
		return err
	}

	if dls != nil {
		ctx.JSON(http.StatusOK, dls)
	} else {
		ctx.JSON(http.StatusOK, emptyArray)
	}

	return nil
}

type downloadQuery struct {
	projectID int
	useSemVer bool

	changelog bool

	builder *db.SQLBuilder
}

func (a *API) createDownloadQuery(ctx *macaron.Context, project maven.Identifier) (*downloadQuery, error) {
	q := new(downloadQuery)

	var lastUpdated time.Time

	// Lookup project
	err := a.DB.QueryRow("SELECT project_id, use_semver, last_updated FROM projects "+
		"WHERE group_id = $1 AND artifact_id = $2;",
		project.GroupID, project.ArtifactID).Scan(&q.projectID, &q.useSemVer, &lastUpdated)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, httperror.NotFound("Unknown project")
		}
		return nil, httperror.InternalError("Database error (failed to lookup project)", err)
	}

	if a.Start.After(lastUpdated) {
		lastUpdated = a.Start
	}

	setLastModified(ctx, lastUpdated)
	if modifiedSince(ctx, lastUpdated) {
		q.builder = db.NewSQLBuilder()
		return q, nil
	}

	return nil, httperror.NotModified // Up-to-date
}

func (q *downloadQuery) init(dependencies bool) {
	q.builder.Append("SELECT download_id, build_types.name, downloads.version, snapshot_version, published, commit, label")

	if q.changelog {
		q.builder.Append(", changelog")
	}

	q.builder.Append(" FROM downloads JOIN build_types USING(build_type_id)")

	if dependencies {
		q.builder.Append(" JOIN dependencies USING(download_id)")
	}

	q.builder.Parameter(" WHERE project_id = ", q.projectID)
}

func (a *API) filterDownloads(ctx *macaron.Context, project maven.Identifier, extended bool, label string) ([]*download, error) {
	q, err := a.createDownloadQuery(ctx, project)
	if err != nil {
		return nil, err
	}

	// Parse filter query parameters
	buildType := ctx.Query("type")
	version := ctx.Query("version")

	if label == "" {
		label = ctx.Query("label")
	}

	// Get possible dependencies
	rows, err := a.DB.Query("SELECT DISTINCT name FROM dependencies "+
		"JOIN downloads USING (download_id)"+
		"WHERE project_id = $1;", q.projectID)
	if err != nil {
		return nil, httperror.InternalError("Database error (failed to lookup dependencies)", err)
	}

	var dependencies [][2]string
	for rows.Next() {
		var name string
		err = rows.Scan(&name)
		if err != nil {
			return nil, httperror.InternalError("Database error (failed to read dependency)", err)
		}

		if val := ctx.Query(name); val != "" {
			dependencies = append(dependencies, [2]string{name, val})
		}
	}

	since := queryIf(ctx, "since", extended)
	until := queryIf(ctx, "until", extended)

	q.changelog = extended && queryBool(ctx, "changelog")

	limit := 1
	if extended {
		limit = ctx.QueryInt("limit")
		if limit <= 0 {
			limit = 10
		} else if limit > 100 {
			limit = 100
		}
	}

	// If since is defined we need an extra outer query to order the rows DESC
	// (We need ASC to limit the results correctly)
	if since != "" {
		q.builder.Append("SELECT * FROM (")
	}

	q.init(dependencies != nil)

	if buildType != "" {
		q.builder.Parameter(" AND build_types.name = ", buildType)
	}

	// Version filter is only supported for projects with semantic versioning
	if q.useSemVer && version != "" {
		version = strings.Trim(version, "%_")

		if strings.Count(version, ".") < 3 {
			version += "."
		}

		version += "%"
		q.builder.Parameter(" AND version LIKE ", version)
	}

	if label != "" {
		q.builder.Parameter(" AND label = ", label)
	}

	for _, dep := range dependencies {
		q.builder.Parameter(" AND dependencies.name = ", dep[0])
		q.builder.Parameter(" AND dependencies.version = ", dep[1])
	}

	if since != "" {
		q.builder.Parameter(" AND published > ", since)
	}

	if until != "" {
		q.builder.Parameter(" AND published < ", until)
	}

	q.builder.Append(" ORDER BY published ")

	if since == "" {
		q.builder.Append("DESC ")
	} else {
		q.builder.Append("ASC ")
	}

	q.builder.Parameter("LIMIT ", limit)

	if since != "" {
		// Finish outer query
		q.builder.Append(") AS d ORDER BY published DESC")
	}

	return q.Read(a, project)
}

func (q *downloadQuery) Read(a *API, project maven.Identifier) ([]*download, error) {
	q.builder.End()

	rows, err := a.DB.Query(q.builder.String(), q.builder.Args()...)
	if err != nil {
		return nil, httperror.InternalError("Database error (failed to lookup downloads)", err)
	}

	var downloadIDs []int64
	var downloadsSlice []*download
	downloadsMap := make(map[int]*download)

	for rows.Next() {
		var id int
		dl := &download{Dependencies: make(map[string]string), Artifacts: make(map[string]*artifact)}
		var changelogJSON []byte

		if q.changelog {
			err = rows.Scan(&id, &dl.Type, &dl.Version, &dl.snapshotVersion, &dl.Published, &dl.Commit, &dl.Label,
				&changelogJSON)
		} else {
			err = rows.Scan(&id, &dl.Type, &dl.Version, &dl.snapshotVersion, &dl.Published, &dl.Commit, &dl.Label)
		}

		if err != nil {
			return nil, httperror.InternalError("Database error (failed to read downloads)", err)
		}

		dl.Changelog = json.RawMessage(changelogJSON)

		downloadIDs = append(downloadIDs, int64(id))
		downloadsSlice = append(downloadsSlice, dl)
		downloadsMap[id] = dl
	}

	if downloadsSlice == nil {
		// No downloads available
		return nil, nil
	}

	// Get download dependencies
	rows, err = a.DB.Query("SELECT download_id, name, version FROM dependencies "+
		"WHERE download_id = ANY($1);", pq.Array(downloadIDs))
	if err != nil {
		return nil, httperror.InternalError("Database error (failed to lookup dependencies)", err)
	}

	for rows.Next() {
		var downloadID int
		var name, version string
		err = rows.Scan(&downloadID, &name, &version)
		if err != nil {
			return nil, httperror.InternalError("Database error (failed to read dependencies)", err)
		}

		downloadsMap[downloadID].Dependencies[name] = version
	}

	// Get download artifacts
	rows, err = a.DB.Query("SELECT download_id, classifier, extension, size, sha1, md5 FROM artifacts "+
		"WHERE download_id = ANY($1);", pq.Array(downloadIDs))
	if err != nil {
		return nil, httperror.InternalError("Database error (failed to lookup artifacts)", err)
	}

	urlPrefix := a.Repo + strings.Replace(project.GroupID, ".", "/", -1) + "/" + project.ArtifactID + "/"

	for rows.Next() {
		var downloadID int
		artifact := new(artifact)
		var classifier, extension string

		err = rows.Scan(&downloadID, &classifier, &extension, &artifact.Size, &artifact.SHA1, &artifact.MD5)
		if err != nil {
			return nil, httperror.InternalError("Database error (failed to read artifacts)", err)
		}

		dl := downloadsMap[downloadID]

		artifact.URL = urlPrefix + defaultWhenNil(dl.snapshotVersion, dl.Version) + "/" +
			project.ArtifactID + "-" + dl.Version

		if classifier != "" {
			artifact.URL += "-" + classifier
		}

		artifact.URL += "." + extension
		dl.Artifacts[classifier] = artifact
	}

	return downloadsSlice, nil
}

func queryIf(ctx *macaron.Context, key string, extended bool) string {
	if extended {
		return ctx.Query(key)
	}
	return ""
}

func queryBool(ctx *macaron.Context, key string) bool {
	_, ok := ctx.Req.Form[key]
	return ok && ctx.QueryBool(key)
}

func defaultWhenNil(a *string, b string) string {
	if a == nil {
		return b
	}
	return *a
}
