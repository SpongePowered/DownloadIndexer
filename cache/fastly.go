package cache

import (
	"errors"
	"fmt"
	"github.com/SpongePowered/SpongeDownloads/maven"
	"net/http"
	"strings"
)

const (
	// Cache for up to 1 month, serve stale content for up to 30 seconds when updating/a week when an error occurs
	fastlyCacheControlHeader = "Surrogate-Control"
	fastlyCacheControl       = "max-age=2628000, stale-while-revalidate=30, stale-if-error=604800"

	fastlyKeys = "Surrogate-Key"

	fastlyAPI       = "https://api.fastly.com/service/%s/%s"
	fastlyKeyHeader = "Fastly-Key"

	fastlyPurgeAll = "purge_all"
	fastlyPurgeKey = "purge/"

	fastlySoftPurgeHeader = "Fastly-Soft-Purge"

	configKeySeparator = '/'
)

var errInvalidFormat = errors.New("Fastly usage: fastly:API_KEY/SERVICE_ID")

type FastlyCache struct {
	APIKey    string
	ServiceID string
}

func (c *FastlyCache) AddHeaders(header http.Header) {
	header.Add(fastlyCacheControlHeader, fastlyCacheControl)
}

func (c *FastlyCache) AddProjectHeaders(header http.Header, project maven.Identifier) {
	header.Add(fastlyKeys, encodeProject(project))
}

func (c *FastlyCache) purge(path string) error {
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf(fastlyAPI, c.ServiceID, path), nil)
	if err != nil {
		return err
	}

	req.Header.Add(fastlyKeyHeader, c.APIKey)
	req.Header.Add(fastlySoftPurgeHeader, "1")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s %s -> %s", req.Method, req.URL, resp.Status)
	}

	return nil
}

func (c *FastlyCache) PurgeAll() error {
	return c.purge(fastlyPurgeAll)
}

func (c *FastlyCache) PurgeProject(project maven.Identifier) error {
	return c.purge(fastlyPurgeKey + encodeProject(project))
}

func encodeProject(project maven.Identifier) string {
	return project.GroupID + "/" + project.ArtifactID
}

func parseFastly(config string) (Cache, error) {
	pos := strings.IndexByte(config, configKeySeparator)
	if pos == -1 {
		return nil, errInvalidFormat
	}

	return &FastlyCache{config[:pos], config[pos+1:]}, nil
}
