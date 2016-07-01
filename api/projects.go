package api

import (
	"bytes"
	"github.com/Minecrell/SpongeDownloads/db"
	"gopkg.in/macaron.v1"
	"net/http"
	"sort"
	"strconv"
)

type project struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	URL        string `json:"url"`
	GroupID    string `json:"groupId"`
	ArtifactID string `json:"artifactId"`

	BuildTypes []*buildType     `json:"buildTypes,omitempty"`
	Minecraft  minecraftSupport `json:"minecraft"`
}

type buildType struct {
	id        int
	Name      string `json:"name"`
	Minecraft string `json:"minecraft,omitempty"`
}

type minecraftSupport struct {
	Current     minecraftVersions `json:"current"`
	Unsupported minecraftVersions `json:"unsupported"`
}

func (a *API) GetProjects(ctx *macaron.Context) {
	rows, err := a.db.Query("SELECT identifier FROM projects;")
	if err != nil {
		panic(err)
	}

	ctx.JSON(http.StatusOK, db.ReadRows(rows))
}

func (a *API) GetProject(ctx *macaron.Context) {
	identifier := ctx.Params("project")

	var p project
	var projectID uint

	row := a.db.QueryRow("SELECT * FROM projects WHERE identifier = $1;", identifier)
	err := row.Scan(&projectID, &p.ID, &p.Name, &p.URL, &p.GroupID, &p.ArtifactID)
	if err != nil {
		panic(err)
	}

	// TODO: I know this sucks, please PR improvements

	rows, err := a.db.Query("SELECT id, name, main FROM branches WHERE project_id = $1 AND NOT obsolete;", projectID)
	if err != nil {
		panic(err)
	}

	var ids bytes.Buffer
	ids.WriteByte('(')

	first := true

	for rows.Next() {
		var id int
		var name string
		var main bool

		err = rows.Scan(&id, &name, &main)
		if err != nil {
			panic(err)
		}

		if first {
			first = false
		} else {
			ids.WriteByte(',')
		}

		ids.WriteString(strconv.Itoa(id))

		if main {
			p.BuildTypes = append(p.BuildTypes, &buildType{id: id, Name: name})
		}
	}

	ids.WriteByte(')')
	rows.Close()

	rows, err = a.db.Query("SELECT branch_id, minecraft FROM downloads " +
		"WHERE (branch_id, published) IN (" +
		"SELECT branch_id, MAX(published) FROM downloads " +
		"WHERE branch_id IN " + ids.String() + " GROUP BY branch_id);")
	if err != nil {
		panic(err)
	}

	for rows.Next() {
		var id int
		var minecraft string

		err = rows.Scan(&id, &minecraft)
		if err != nil {
			panic(err)
		}

		p.Minecraft.Current = append(p.Minecraft.Current, minecraft)

		for _, t := range p.BuildTypes {
			if t.id == id {
				t.Minecraft = minecraft
			}
		}
	}

	rows.Close()

	rows, err = a.db.Query("SELECT minecraft FROM downloads WHERE project_id = $1 GROUP BY minecraft;", projectID)
	if err != nil {
		panic(err)
	}

rows:
	for rows.Next() {
		var minecraft string

		err = rows.Scan(&minecraft)
		if err != nil {
			panic(err)
		}

		for _, m := range p.Minecraft.Current {
			if m == minecraft {
				continue rows
			}
		}

		p.Minecraft.Unsupported = append(p.Minecraft.Unsupported, minecraft)
	}

	sort.Sort(p.Minecraft.Current)
	sort.Sort(p.Minecraft.Unsupported)

	ctx.JSON(http.StatusOK, p)
}
