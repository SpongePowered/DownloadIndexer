package api

import (
	"database/sql"
	"github.com/Minecrell/SpongeDownloads/downloads"
	"github.com/Minecrell/SpongeDownloads/maven"
	"github.com/lib/pq"
	"gopkg.in/macaron.v1"
	"net/http"
	"sort"
)

type project struct {
	Name     string `json:"name"`
	PluginID string `json:"pluginId"`

	GitHub struct {
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
	} `json:"github"`

	BuildTypes map[string]*buildType `json:"buildTypes,omitempty"`

	Versions     versions            `json:"versions,omitempty"`
	Dependencies map[string]versions `json:"dependencies,omitempty"`
}

type buildType struct {
	id int

	Latest      *build `json:"latest,omitempty"`
	Recommended *build `json:"recommended,omitempty"`

	AllowsPromotion bool `json:"allowsPromotion"`
}

type build struct {
	id int

	Version      string            `json:"version"`
	Dependencies map[string]string `json:"dependencies,omitempty"`
}

func (a *API) GetProject(ctx *macaron.Context, c maven.Identifier) error {
	p := project{BuildTypes: make(map[string]*buildType), Dependencies: make(map[string]versions)}
	var projectID int
	var useSemVer bool

	err := a.DB.QueryRow("SELECT project_id, name, plugin_id, github_owner, github_repo, use_semver FROM projects "+
		"WHERE group_id = $1 AND artifact_id = $2;",
		c.GroupID, c.ArtifactID).Scan(&projectID, &p.Name, &p.PluginID, &p.GitHub.Owner, &p.GitHub.Repo, &useSemVer)
	if err != nil {
		if err == sql.ErrNoRows {
			return downloads.NotFound("Unknown project")
		}
		return downloads.InternalError("Database error (failed to lookup project)", err)
	}

	// Get build types
	rows, err := a.DB.Query("SELECT build_type_id, name, allows_promotion FROM build_types "+
		"JOIN project_build_types USING(build_type_id) WHERE project_id = $1;", projectID)
	if err != nil {
		return downloads.InternalError("Database error (failed to lookup build types)", err)
	}

	for rows.Next() {
		bt := new(buildType)
		var name string
		err = rows.Scan(&bt.id, &name, &bt.AllowsPromotion)
		if err != nil {
			return downloads.InternalError("Database error (failed to read build type)", err)
		}

		p.BuildTypes[name] = bt
	}

	// Get latest download for each build type
	rows, err = a.DB.Query("SELECT build_type_id, label, download_id, version FROM downloads "+
		"WHERE (build_type_id, coalesce(label, ''), published) IN ("+
		"SELECT build_type_id, coalesce(label, ''), MAX(published) FROM downloads "+
		"WHERE project_id = $1 GROUP BY build_type_id, label)"+
		"ORDER BY published DESC;", projectID)
	if err != nil {
		return downloads.InternalError("Database error (failed to get latest downloads)", err)
	}

	downloadIDs := make([]int64, 0, len(p.BuildTypes))

rows:
	for rows.Next() {
		var buildTypeID, downloadID int
		var version string
		var label *string

		err = rows.Scan(&buildTypeID, &label, &downloadID, &version)
		if err != nil {
			return downloads.InternalError("Database error (failed to read latest download)", err)
		}

		for _, bt := range p.BuildTypes {
			if bt.id == buildTypeID {
				build := &build{
					id:           downloadID,
					Version:      version,
					Dependencies: make(map[string]string),
				}

				if label == nil {
					if bt.Latest == nil {
						bt.Latest = build
					}
				} else if *label == "recommended" {
					bt.Recommended = build
					if bt.Latest == nil {
						bt.Latest = build // Use recommended as fallback for latest
					}
				}

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
			switch {
			case bt.Latest != nil && bt.Latest.id == downloadID:
				bt.Latest.Dependencies[name] = version
			case bt.Recommended != nil && bt.Recommended.id == downloadID:
				bt.Recommended.Dependencies[name] = version
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

	for _, deps := range p.Dependencies {
		sort.Sort(deps)
	}

	if useSemVer {
		// Add all versions
		rows, err = a.DB.Query("SELECT DISTINCT split_part(version, '-', 1) FROM downloads "+
			"WHERE project_id = $1;", projectID)
		if err != nil {
			return downloads.InternalError("Database error (failed to lookup versions)", err)
		}

		for rows.Next() {
			var version string
			err = rows.Scan(&version)
			if err != nil {
				return downloads.InternalError("Database error (failed to read version)", err)
			}

			p.Versions = append(p.Versions, version)
		}

		sort.Sort(p.Versions)
	}

	ctx.JSON(http.StatusOK, p)
	return nil
}
