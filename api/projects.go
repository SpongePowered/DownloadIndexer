package api

import (
	"github.com/SpongePowered/DownloadIndexer/httperror"
	"github.com/SpongePowered/DownloadIndexer/maven"
	"gopkg.in/macaron.v1"
	"net/http"
	"time"
)

func (a *API) GetProjects(ctx *macaron.Context) error {
	rows, err := a.DB.Query("SELECT group_id, artifact_id, last_updated FROM projects;")
	if err != nil {
		return httperror.InternalError("Database error (failed to query projects)", err)
	}

	var projects []maven.Identifier
	maxLastUpdated := a.Start

	for rows.Next() {
		var project maven.Identifier
		var lastUpdated time.Time
		err = rows.Scan(&project.GroupID, &project.ArtifactID, &maxLastUpdated)
		if err != nil {
			return httperror.InternalError("Database error (failed to read project)", err)
		}

		projects = append(projects, project)

		if lastUpdated.After(maxLastUpdated) {
			maxLastUpdated = lastUpdated
		}
	}

	setLastModified(ctx, maxLastUpdated)

	if modifiedSince(ctx, maxLastUpdated) {
		ctx.JSON(http.StatusOK, projects)
	}

	return nil
}
