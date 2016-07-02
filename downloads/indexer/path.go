package indexer

import (
	"errors"
	"github.com/Minecrell/SpongeDownloads/downloads/db"
	"strings"
)

func parsePath(path string) (v version, a artifactType, err error) {
	pos := len(path)

	filename, err := findPathSegment(path, &pos)
	if err != nil {
		return
	}

	v.version, err = findPathSegment(path, &pos)
	if err != nil {
		return
	}

	v.artifactID, err = findPathSegment(path, &pos)
	if err != nil {
		return
	}

	v.groupID = strings.Replace(path[:pos], "/", ".", -1)

	if !strings.HasPrefix(filename, v.artifactID) || filename[len(v.artifactID)] != '-' {
		err = errors.New("Invalid filename (missing artifact ID): " + filename)
		return
	}

	filename = filename[len(v.artifactID)+1:]

	if strings.HasSuffix(v.version, "-SNAPSHOT") {
		// Special handling for snapshots, string should start with the version, without SNAPSHOT
		l := len(v.version) - 8

		if !strings.HasPrefix(filename, v.version[:l]) {
			err = errors.New("Invalid filename (missing version): " + filename)
			return
		}

		// Find end of snapshot version, starting with the version prefix
		// + 16 for the datetime
		end := findNonNumeric(filename, l+16)
		v.snapshotVersion = db.ToNullString(filename[:end])

		filename = filename[end:]
	} else {
		if !strings.HasPrefix(filename, v.version) {
			err = errors.New("Invalid filename (missing version): " + filename)
			return
		}

		filename = filename[len(v.version):]
	}

	if filename[0] == '-' {
		// Classifier, find end
		end := strings.LastIndexByte(filename, '.')
		a.classifier = db.ToNullString(filename[1:end])

		filename = filename[end:]
	} else if filename[0] != '.' {
		err = errors.New("Invalid filename (invalid version): " + filename)
		return
	}

	a.extension = filename[1:]
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
