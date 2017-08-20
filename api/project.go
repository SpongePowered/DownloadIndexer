package api

import (
	"database/sql"
	"github.com/SpongePowered/SpongeDownloads/httperror"
	"github.com/SpongePowered/SpongeDownloads/maven"
	"github.com/lib/pq"
	"gopkg.in/macaron.v1"
	"net/http"
	"sort"
	"time"
)

type project struct {
	Name     string `json:"name"`
	PluginID string `json:"pluginId"`

	GitHub struct {
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
	} `json:"github"`

	Branches map[string]*branch `json:"branches,omitempty"`

	Versions     versions            `json:"versions,omitempty"`
	Dependencies map[string]versions `json:"dependencies,omitempty"`
}

type branch struct {
	id int

	BuildType   string    `json:"buildType"`
	Created     time.Time `json:"created"`
	Latest      *build    `json:"latest,omitempty"`
	Recommended *build    `json:"recommended,omitempty"`
}

type build struct {
	id int

	Version      string            `json:"version"`
	Dependencies map[string]string `json:"dependencies,omitempty"`
}

func (a *API) GetProject(ctx *macaron.Context, c maven.Identifier) error {
	p := project{Branches: make(map[string]*branch), Dependencies: make(map[string]versions)}
	var projectID int
	var useSemVer bool
	var lastUpdated time.Time

	err := a.DB.QueryRow("SELECT project_id, name, plugin_id, github_owner, github_repo, use_semver, last_updated "+
		"FROM projects "+
		"WHERE group_id = $1 AND artifact_id = $2;", c.GroupID, c.ArtifactID).Scan(&projectID,
		&p.Name, &p.PluginID, &p.GitHub.Owner, &p.GitHub.Repo, &useSemVer, &lastUpdated)
	if err != nil {
		if err == sql.ErrNoRows {
			return httperror.NotFound("Unknown project")
		}
		return httperror.InternalError("Database error (failed to lookup project)", err)
	}

	if a.Start.After(lastUpdated) {
		lastUpdated = a.Start
	}

	setLastModified(ctx, lastUpdated)

	if !modifiedSince(ctx, lastUpdated) {
		return nil
	}

	// Get all active branches
	rows, err := a.DB.Query("SELECT branch_id, branches.name, build_types.name, created FROM branches "+
		"JOIN build_types USING(build_type_id) WHERE project_id = $1 AND active;", projectID)
	if err != nil {
		return httperror.InternalError("Database error (failed to lookup branches)", err)
	}

	for rows.Next() {
		b := new(branch)
		var name string
		err = rows.Scan(&b.id, &name, &b.BuildType, &b.Created)
		if err != nil {
			return httperror.InternalError("Database error (failed to read branch)", err)
		}

		p.Branches[name] = b
	}

	// Get latest download for each branch
	rows, err = a.DB.Query("SELECT branch_id, label, download_id, version FROM downloads "+
		"WHERE (branch_id, coalesce(label, ''), published) IN ("+
		"SELECT branch_id, coalesce(label, ''), MAX(published) FROM downloads "+
		"WHERE project_id = $1 GROUP BY branch_id, label)"+
		"ORDER BY published DESC;", projectID)
	if err != nil {
		return httperror.InternalError("Database error (failed to get latest downloads)", err)
	}

	downloadIDs := make([]int64, 0, len(p.Branches))

rows:
	for rows.Next() {
		var buildTypeID, downloadID int
		var version string
		var label *string

		err = rows.Scan(&buildTypeID, &label, &downloadID, &version)
		if err != nil {
			return httperror.InternalError("Database error (failed to read latest download)", err)
		}

		for _, b := range p.Branches {
			if b.id == buildTypeID {
				build := &build{
					id:           downloadID,
					Version:      version,
					Dependencies: make(map[string]string),
				}

				if label == nil {
					if b.Latest == nil {
						b.Latest = build
					}
				} else if *label == "recommended" {
					b.Recommended = build
					if b.Latest == nil {
						b.Latest = build // Use recommended as fallback for latest
					}
				}

				downloadIDs = append(downloadIDs, int64(downloadID))
				continue rows
			}
		}

		// Skip download from inactive branch
		// TODO: Exclude them from the query above?
	}

	// Get dependencies for latest builds
	rows, err = a.DB.Query("SELECT * FROM dependencies WHERE download_ID = ANY($1);", pq.Array(downloadIDs))
	if err != nil {
		return httperror.InternalError("Database error (failed to get latest dependencies)", err)
	}

	for rows.Next() {
		var downloadID int
		var name, version string
		err = rows.Scan(&downloadID, &name, &version)
		if err != nil {
			return httperror.InternalError("Database error (failed to read latest dependency)", err)
		}

		for _, b := range p.Branches {
			switch {
			case b.Latest != nil && b.Latest.id == downloadID:
				b.Latest.Dependencies[name] = version
			case b.Recommended != nil && b.Recommended.id == downloadID:
				b.Recommended.Dependencies[name] = version
			}
		}
	}

	// Get all available dependencies
	rows, err = a.DB.Query("SELECT DISTINCT name, split_part(dependencies.version, '-', 1) FROM dependencies "+
		"JOIN downloads USING(download_id) WHERE project_id = $1;", projectID)
	if err != nil {
		return httperror.InternalError("Database error (failed to lookup dependency versions)", err)
	}

	for rows.Next() {
		var name, version string
		err = rows.Scan(&name, &version)
		if err != nil {
			return httperror.InternalError("Database error (failed to read dependency version)", err)
		}

		p.Dependencies[name] = append(p.Dependencies[name], version)
	}

	for _, deps := range p.Dependencies {
		sort.Sort(deps)
	}

	if useSemVer {
		// Add all versions
		rows, err = a.DB.Query("SELECT DISTINCT split_part(version, '-', 1) FROM downloads "+
			"WHERE project_id = $1;", projectID)
		if err != nil {
			return httperror.InternalError("Database error (failed to lookup versions)", err)
		}

		for rows.Next() {
			var version string
			err = rows.Scan(&version)
			if err != nil {
				return httperror.InternalError("Database error (failed to read version)", err)
			}

			p.Versions = append(p.Versions, version)
		}

		sort.Sort(p.Versions)
	}

	ctx.JSON(http.StatusOK, p)
	return nil
}
