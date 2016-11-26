package indexer

import (
	"errors"
	"github.com/SpongePowered/SpongeDownloads/maven"
	"strings"
)

type fileType int

const (
	mavenMetadataFile = "maven-metadata.xml"
	md5Extension      = ".md5"
	sha1Extension     = ".sha1"

	snapshotSuffix = "-SNAPSHOT"

	file fileType = iota
	md5File
	sha1File
)

type path struct {
	maven.Identifier
	t fileType

	metadata bool

	version        string
	displayVersion string

	snapshot bool

	artifact artifactType
}

type artifactType struct {
	classifier string
	extension  string
}

func parsePath(path string, parseArtifact bool) (p path, err error) {
	switch {
	case strings.HasSuffix(path, md5Extension):
		p.t = md5File
		// Strip .md5 from path for further processing
		path = path[:len(path)-len(md5Extension)]
	case strings.HasSuffix(path, sha1Extension):
		p.t = sha1File
		// Strip .sha1 from path for further processing
		path = path[:len(path)-len(sha1Extension)]
	default:
		p.t = file
	}

	pos := len(path)

	filename, err := findPathSegment(path, &pos)
	if err != nil {
		return
	}

	next, err := findPathSegment(path, &pos)
	if err != nil {
		return
	}

	p.metadata = filename == mavenMetadataFile

	if !p.metadata || strings.HasSuffix(next, snapshotSuffix) {
		p.version, p.displayVersion = next, next
		next, err = findPathSegment(path, &pos)
		if err != nil {
			return
		}
	}

	p.ArtifactID = next
	p.GroupID = strings.Replace(path[:pos], "/", ".", -1)

	if parseArtifact && !p.metadata {
		if !strings.HasPrefix(filename, p.ArtifactID) || filename[len(p.ArtifactID)] != '-' {
			err = errors.New("Invalid filename (missing artifact ID): " + filename)
			return
		}

		filename = filename[len(p.ArtifactID)+1:]

		if strings.HasSuffix(p.version, snapshotSuffix) {
			// Special handling for snapshots, string should start with the version, without SNAPSHOT
			l := len(p.version) - len(snapshotSuffix) + 1

			if !strings.HasPrefix(filename, p.version[:l]) {
				err = errors.New("Invalid filename (missing version): " + filename)
				return
			}

			// Find end of snapshot version, starting with the version prefix
			// + 16 for the datetime
			end := findNonNumeric(filename, l+16)
			p.displayVersion = filename[:end]
			p.snapshot = true

			filename = filename[end:]
		} else {
			if !strings.HasPrefix(filename, p.version) {
				err = errors.New("Invalid filename (missing version): " + filename)
				return
			}

			filename = filename[len(p.version):]
		}

		if filename[0] == '-' {
			// Classifier, find end
			end := strings.LastIndexByte(filename, '.')
			p.artifact.classifier = filename[1:end]

			filename = filename[end:]
		} else if filename[0] != '.' {
			err = errors.New("Invalid filename (invalid version): " + filename)
			return
		}

		p.artifact.extension = filename[1:]
	}

	return
}

func findNonNumeric(s string, start int) int {
	for i := start; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return i
		}
	}

	return -1
}

func findPathSegment(s string, pos *int) (result string, err error) {
	for i := *pos - 1; i >= 0; i-- {
		if s[i] == '/' {
			result = s[i+1 : *pos]
			*pos = i
			return result, nil
		}
	}

	err = errors.New("No path segment found in " + result)
	return
}

func substringBefore(s string, c byte) string {
	i := strings.IndexByte(s, c)
	if i >= 0 {
		return s[:i]
	}

	return s
}
