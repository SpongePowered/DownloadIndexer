package mcmod

import (
	"bytes"
	"encoding/json"
	"io"
)

const (
	MetadataFileName = "mcmod.info"
	versionSeparator = '@'
)

type Metadata struct {
	ID string `json:"modid"`
	//Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`

	//Description string `json:"description,omitempty"`
	//URL         string `json:"url,omitempty"`

	//Authors []string `json:"authors,omitempty"`

	Dependencies []Dependency `json:"requiredMods,omitempty"`
	//LoadBefore   []Dependency `json:"dependants,omitempty"`
	//LoadAfter    []Dependency `json:"dependencies,omitempty"`
}

type Dependency struct {
	ID      string
	Version string
}

func (d *Dependency) MarshalText() ([]byte, error) {
	if d.Version != "" {
		return []byte(d.ID + string(versionSeparator) + d.Version), nil
	}

	return []byte(d.ID), nil
}

func (d *Dependency) UnmarshalText(text []byte) error {
	pos := bytes.IndexByte(text, versionSeparator)
	if pos >= 0 {
		d.ID, d.Version = string(text[:pos]), string(text[pos+1:])
	} else {
		d.ID = string(text)
	}

	return nil
}

func ReadMetadataBytes(bytes []byte) ([]*Metadata, error) {
	var meta []*Metadata
	return meta, json.Unmarshal(bytes, &meta)
}

func ReadMetadata(reader io.Reader) ([]*Metadata, error) {
	var meta []*Metadata
	return meta, json.NewDecoder(reader).Decode(&meta)
}
