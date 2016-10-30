package api

import (
	"database/sql"
	"github.com/Minecrell/SpongeDownloads/downloads"
	"github.com/Minecrell/SpongeDownloads/downloads/maven"
	"github.com/lib/pq"
	"gopkg.in/macaron.v1"
	"net/http"
)

type project struct {
	maven.Identifier

	Name     string `json:"name"`
	PluginID string `json:"pluginId"`

	GitHub struct {
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
	} `json:"github"`

	BuildTypes   map[string]*buildType `json:"buildTypes,omitempty"`
	Dependencies map[string][]string   `json:"dependencies,omitempty"`

	Snapshots bool `json:"snapshots"`
}

type buildType struct {
	id               int
	latestDownloadID int

	Dependencies map[string]string `json:"dependencies"`
}

func (a *API) GetProject(ctx *macaron.Context, c maven.Identifier) error {
	p := project{BuildTypes: make(map[string]*buildType), Dependencies: make(map[string][]string)}
	var projectID int

	err := a.DB.QueryRow("SELECT * FROM projects WHERE group_id = $1 AND artifact_id = $2;",
		c.GroupID, c.ArtifactID).Scan(&projectID, &p.Name, &p.GroupID, &p.ArtifactID, &p.PluginID,
		&p.GitHub.Owner, &p.GitHub.Repo, &p.Snapshots)
	if err != nil {
		if err == sql.ErrNoRows {
			return downloads.NotFound("Unknown project")
		}
		return downloads.InternalError("Database error (failed to lookup project)", err)
	}

	// Get build types
	rows, err := a.DB.Query("SELECT build_type_id, name FROM build_types "+
		"JOIN project_build_types USING(build_type_id) WHERE project_id = $1;", projectID)
	if err != nil {
		return downloads.InternalError("Database error (failed to lookup build types)", err)
	}

	for rows.Next() {
		bt := &buildType{Dependencies: make(map[string]string)}
		var name string
		err = rows.Scan(&bt.id, &name)
		if err != nil {
			return downloads.InternalError("Database error (failed to read build type)", err)
		}

		p.BuildTypes[name] = bt
	}

	// Get latest download for each build type
	rows, err = a.DB.Query("SELECT build_type_id, download_id FROM downloads "+
		"WHERE (build_type_id, published) IN ("+
		"SELECT build_type_id, MAX(published) FROM downloads "+
		"WHERE project_id = $1 GROUP BY build_type_id"+
		");", projectID)
	if err != nil {
		return downloads.InternalError("Database error (failed to get latest downloads)", err)
	}

	downloadIDs := make([]int64, 0, len(p.BuildTypes))

rows:
	for rows.Next() {
		var buildTypeID, downloadID int
		err = rows.Scan(&buildTypeID, &downloadID)
		if err != nil {
			return downloads.InternalError("Database error (failed to read latest download)", err)
		}

		for _, bt := range p.BuildTypes {
			if bt.id == buildTypeID {
				bt.latestDownloadID = downloadID
				downloadIDs = append(downloadIDs, int64(downloadID))
				continue rows
			}
		}

		return downloads.InternalError("Found unknown build type ID", nil)
	}

	// Get dependencies for latest builds
	rows, err = a.DB.Query("SELECT * FROM dependencies WHERE download_ID = ANY($1);", pq.Array(downloadIDs))
	if err != nil {
		return downloads.InternalError("Database error (failed to get latest dependencies)", err)
	}

	for rows.Next() {
		var downloadID int
		var name, version string
		err = rows.Scan(&downloadID, &name, &version)
		if err != nil {
			return downloads.InternalError("Database error (failed to read latest dependency)", err)
		}

		for _, bt := range p.BuildTypes {
			if bt.latestDownloadID == downloadID {
				bt.Dependencies[name] = version
			}
		}
	}

	// Get all available dependencies
	rows, err = a.DB.Query("SELECT DISTINCT name, split_part(dependencies.version, '-', 1) FROM dependencies "+
		"JOIN downloads USING(download_id) WHERE project_id = $1;", projectID)
	if err != nil {
		return downloads.InternalError("Database error (failed to lookup dependency versions)", err)
	}

	for rows.Next() {
		var name, version string
		err = rows.Scan(&name, &version)
		if err != nil {
			return downloads.InternalError("Database error (failed to read dependency version)", err)
		}

		p.Dependencies[name] = append(p.Dependencies[name], version)
	}

	ctx.JSON(http.StatusOK, p)
	return nil
}
