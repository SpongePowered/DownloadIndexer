package cache

import (
	"errors"
	"github.com/SpongePowered/SpongeDownloads/maven"
	"net/http"
	"strings"
)

const cacheTypeSeparator = ':'

type Cache interface {
	AddHeaders(header http.Header)
	AddProjectHeaders(header http.Header, project maven.Identifier)

	PurgeAll() error
	PurgeProject(project maven.Identifier) error
}

func Create(config string) (Cache, error) {
	pos := strings.IndexByte(config, cacheTypeSeparator)
	if pos == -1 {
		return nil, errors.New("Invalid cache config: " + config)
	}

	cacheType := config[:pos]
	config = config[pos+1:]

	switch cacheType {
	case "fastly":
		return parseFastly(config)
	default:
		return nil, errors.New("Unsupported cache type: " + cacheType)
	}
}
