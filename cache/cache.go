package cache

import (
	"errors"
	"github.com/SpongePowered/SpongeDownloads/maven"
	"gopkg.in/macaron.v1"
	"log"
	"net/http"
	"strings"
)

const cacheTypeSeparator = ':'

type Cache interface {
	AddHeaders(header http.Header)
	AddProjectHeaders(header http.Header, project maven.Identifier)

	LogHandler() macaron.Handler

	PurgeAll() bool
	PurgeProject(project maven.Identifier) bool
}

func Create(logger *log.Logger, config string) (Cache, error) {
	pos := strings.IndexByte(config, cacheTypeSeparator)
	if pos == -1 {
		return nil, errors.New("Invalid cache config: " + config)
	}

	cacheType := config[:pos]
	config = config[pos+1:]

	switch cacheType {
	case "fastly":
		return parseFastly(logger, config)
	default:
		return nil, errors.New("Unsupported cache type: " + cacheType)
	}
}
