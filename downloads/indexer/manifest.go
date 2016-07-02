package indexer

import (
	"bufio"
	"io"
	"strings"
)

const manifestPath = "META-INF/MANIFEST.MF"

type manifest map[string]string

func readManifest(reader io.ReadCloser) (result manifest, err error) {
	defer reader.Close()

	result = make(manifest)

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		i := strings.IndexByte(line, ':')
		if i == -1 {
			continue // TODO: Warn about invalid lines?
		}

		key := strings.TrimSpace(line[:i])
		value := strings.TrimSpace(line[i+1:])
		result[key] = value
	}

	err = scanner.Err()
	return
}
