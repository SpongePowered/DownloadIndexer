package indexer

import (
	"archive/zip"
	"bufio"
	"bytes"
	"io"
	"strings"
	"time"
)

const manifestPath = "META-INF/MANIFEST.MF"

type manifest map[string]string

func readManifestFromZip(zipBytes []byte) (m manifest, time time.Time, err error) {
	reader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return
	}

	for _, file := range reader.File {
		if file.Name == manifestPath {
			time = file.ModTime()

			var r io.ReadCloser
			r, err = file.Open()
			if err != nil {
				return
			}

			m, err = readManifest(r)
			if err == nil {
				err = r.Close()
			} else {
				r.Close()
			}
			return
		}
	}

	return
}

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
