package manifest

import (
	"bufio"
	"bytes"
	"io"
)

const (
	JarPath   = "META-INF/MANIFEST.MF"
	separator = ':'
)

type Manifest map[string]string

func Read(reader io.Reader) (Manifest, error) {
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
