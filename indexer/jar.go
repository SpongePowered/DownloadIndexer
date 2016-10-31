package indexer

import (
	"archive/zip"
	"bytes"
	"github.com/Minecrell/SpongeDownloads/indexer/manifest"
	"github.com/Minecrell/SpongeDownloads/indexer/meta"
	"time"
)

func readJar(zipBytes []byte, readMeta bool) (m manifest.Manifest, metadata []*meta.PluginMetadata, time time.Time, err error) {
	reader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return
	}

	for _, file := range reader.File {
		switch file.Name {
		case manifest.JarPath:
			time = file.ModTime()
			m, err = readManifest(file)
			if err != nil {
				return
			}

			if !readMeta || metadata != nil {
				return
			}
		case meta.FileName:
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

func readManifest(file *zip.File) (manifest.Manifest, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}

	defer reader.Close()
	return manifest.Read(reader)
}

func readMetadata(file *zip.File) ([]*meta.PluginMetadata, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}

	defer reader.Close()
	return meta.Read(reader)
}
