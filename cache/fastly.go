package cache

import (
	"github.com/SpongePowered/SpongeDownloads/maven"
	"github.com/SpongePowered/SpongeWebGo/cache"
	"github.com/SpongePowered/SpongeWebGo/fastly"
	"log"
	"net/http"
)

type FastlyCache struct {
	*fastly.Cache
}

func (c *FastlyCache) AddHeaders(header http.Header) {
	header.Add(fastly.SurrogateControlHeader, cache.StaticContentOptions)
}

func (c *FastlyCache) AddProjectHeaders(header http.Header, project maven.Identifier) {
	header.Add(fastly.KeyHeader, encodeProject(project))
}

func (c *FastlyCache) PurgeProject(project maven.Identifier) bool {
	return c.PurgeKey(encodeProject(project))
}

func encodeProject(project maven.Identifier) string {
	return project.GroupID + "/" + project.ArtifactID
}

func parseFastly(logger *log.Logger, config string) (Cache, error) {
	c, err := fastly.ParseConfig(logger, config)
	if err != nil {
		return nil, err
	}

	return &FastlyCache{c}, nil
}
