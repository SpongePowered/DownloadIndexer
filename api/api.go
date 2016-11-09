package api

import (
	"github.com/Minecrell/SpongeDownloads/downloads"
	"github.com/Minecrell/SpongeDownloads/maven"
	"gopkg.in/macaron.v1"
	"strings"
)

type API struct {
	*downloads.Module
	Repo string
}

func Create(m *downloads.Manager, repo string) *API {
	// Make sure repo URL ends with a slash
	if repo[len(repo)-1] != '/' {
		repo += "/"
	}

	return &API{m.Module("API"), repo}
}

func (a *API) Setup(m *macaron.Macaron, renderer macaron.Handler) {
	m.Group("/v1", func() {
		m.Get("/projects", a.GetProjects)

		m.Group("/:groupId/:artifactId", func() {
			m.Get("/", a.GetProject)
			m.Get("/downloads", a.GetDownloads)
			m.Get("/downloads/:version", a.GetDownload)
			m.Get("/downloads/recommended", a.GetRecommendedDownload)
		}, parseIdentifier)
	},
		a.InitializeContext,
		macaron.Recovery(),
		AddHeaders,
		renderer)
}

func AddHeaders(ctx *macaron.Context) {
	header := ctx.Header()
	header.Add("Access-Control-Allow-Origin", "*")
}

func parseIdentifier(ctx *macaron.Context) error {
	i := maven.Identifier{ctx.Params("groupId"), ctx.Params("artifactId")}
	if i.GroupID == "" || i.ArtifactID == "" {
		return downloads.BadRequest("Invalid group or artifact ID", nil)
	}

	if strings.IndexByte(i.ArtifactID, '.') != -1 {
		return downloads.BadRequest("Artifact ID cannot contain dots", nil)
	}

	ctx.Map(i)
	ctx.Next()
	return nil
}
