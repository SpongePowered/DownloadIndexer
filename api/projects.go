package api

import (
	"github.com/Minecrell/SpongeDownloads/downloads"
	"github.com/Minecrell/SpongeDownloads/maven"
	"gopkg.in/macaron.v1"
	"net/http"
)

func (a *API) GetProjects(ctx *macaron.Context) error {
	rows, err := a.DB.Query("SELECT group_id, artifact_id FROM projects;")
	if err != nil {
		return downloads.InternalError("Database error (failed to query projects)", err)
	}

	var projects []maven.Identifier
	for rows.Next() {
		var project maven.Identifier
		err = rows.Scan(&project.GroupID, &project.ArtifactID)
		if err != nil {
			return downloads.InternalError("Database error (failed to read project)", err)
		}

		projects = append(projects, project)
	}

	ctx.JSON(http.StatusOK, projects)
	return nil
}
