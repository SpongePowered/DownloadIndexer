package jar

import (
	"bufio"
	"bytes"
	"io"
)

const (
	ManifestPath = "META-INF/MANIFEST.MF"
	separator    = ':'
)

type Manifest map[string]string

func ReadManifest(reader io.Reader) (Manifest, error) {
	m := make(Manifest)

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Bytes()
		i := bytes.IndexByte(line, separator)
		if i == -1 {
			continue // TODO: Warn about invalid lines?
		}

		key := string(bytes.TrimSpace(line[:i]))
		value := string(bytes.TrimSpace(line[i+1:]))
		m[key] = value
	}

	return m, scanner.Err()
}
