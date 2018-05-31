package indexer

import (
	"archive/zip"
	"bytes"
	"github.com/SpongePowered/DownloadIndexer/indexer/jar"
	"github.com/SpongePowered/DownloadIndexer/indexer/mcmod"
	"time"
)

func readJar(zipBytes []byte, readMeta bool) (m jar.Manifest, manifestTime time.Time, metadata []*mcmod.Metadata, err error) {
	reader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return
	}

	for _, file := range reader.File {
		switch file.Name {
		case jar.ManifestPath:
			if file.ModifiedTime != 0 || file.ModifiedDate > 33 {
				// Modification time is set
				manifestTime = file.ModTime()
			}

			m, err = readManifest(file)
			if err != nil {
				return
			}

			if !readMeta || metadata != nil {
				return
			}
		case mcmod.MetadataFileName:
			if readMeta {
				metadata, err = readMetadata(file)
				if err != nil {
					return
				}

				if m != nil {
					return
				}
			}
		}
	}

	return
}

func readManifest(file *zip.File) (jar.Manifest, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}

	defer reader.Close()
	return jar.ReadManifest(reader)
}

func readMetadata(file *zip.File) ([]*mcmod.Metadata, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}

	defer reader.Close()
	return mcmod.ReadMetadata(reader)
}
