package api

import (
	"github.com/SpongePowered/SpongeDownloads/downloads"
	"github.com/SpongePowered/SpongeDownloads/httperror"
	"github.com/SpongePowered/SpongeDownloads/maven"
	"github.com/go-macaron/gzip"
	"gopkg.in/macaron.v1"
	"net/http"
	"strings"
	"time"
)

type API struct {
	*downloads.Module
	Repo string

	Start time.Time
}

func Create(m *downloads.Manager, repo string) *API {
	// Make sure repo URL ends with a slash
	if repo[len(repo)-1] != '/' {
		repo += "/"
	}

	return &API{
		Module: m.Module("API"),
		Repo:   repo,
	}
}

func (a *API) Setup(m *macaron.Macaron, renderer macaron.Handler) {
	m.Group("/v1", func() {
		m.Get("/projects", a.GetProjects)

		m.Group("/:groupId/:artifactId", func() {
			m.Get("/", a.GetProject)
			m.Get("/downloads", a.GetDownloads)
			m.Get("/downloads/:version", a.GetDownload)
			m.Get("/downloads/recommended", a.GetRecommendedDownload)
		}, a.parseIdentifier)
	},
		a.InitializeContext,
		macaron.Recovery(),
		gzip.Gziper(),
		a.addHeaders,
		renderer)

	if a.Cache != nil {
		go func() {
			err := a.Cache.PurgeAll()
			if err != nil {
				a.Log.Println("Failed to purge cache:", err)
			} else {
				a.Log.Println("Cache purged successfully")
			}
		}()
	}

	a.Start = time.Now().UTC().Truncate(time.Second)
}

func (a *API) addHeaders(resp http.ResponseWriter) {
	header := resp.Header()
	header.Add("Access-Control-Allow-Origin", "*")
	header.Add("Cache-Control", "no-cache")

	if a.Cache != nil {
		a.Cache.AddHeaders(header)
	}
}

func parseIfModifiedSince(ctx *macaron.Context) (time.Time, error) {
	return time.Parse(http.TimeFormat, ctx.Req.Header.Get("If-Modified-Since"))
}

func modifiedSince(ctx *macaron.Context, lastUpdated time.Time) bool {
	if modifiedSince, err := parseIfModifiedSince(ctx); err == nil && modifiedSince.Equal(lastUpdated) {
		ctx.Status(http.StatusNotModified)
		return false
	}

	return true
}

func setLastModified(ctx *macaron.Context, lastUpdated time.Time) {
	ctx.Header().Add("Last-Modified", lastUpdated.UTC().Format(http.TimeFormat))
}

func (a *API) parseIdentifier(ctx *macaron.Context) error {
	i := maven.Identifier{ctx.Params("groupId"), ctx.Params("artifactId")}
	if i.GroupID == "" || i.ArtifactID == "" {
		return httperror.BadRequest("Invalid group or artifact ID", nil)
	}

	if strings.IndexByte(i.ArtifactID, '.') != -1 {
		return httperror.BadRequest("Artifact ID cannot contain dots", nil)
	}

	if a.Cache != nil {
		a.Cache.AddProjectHeaders(ctx.Resp.Header(), i)
	}

	ctx.Map(i)
	ctx.Next()
	return nil
}
