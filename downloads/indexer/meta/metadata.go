package meta

import (
	"bytes"
	"encoding/json"
	"io"
)

const (
	FileName         = "mcmod.info"
	versionSeparator = '@'
)

type PluginMetadata struct {
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
	} else {
		return []byte(d.ID), nil
	}
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

/*func ReadBytes(bytes []byte) ([]*PluginMetadata, error) {
	var meta []*PluginMetadata
	return meta, json.Unmarshal(bytes, meta)
}*/

func Read(reader io.Reader) ([]*PluginMetadata, error) {
	var meta []*PluginMetadata
	return meta, json.NewDecoder(reader).Decode(&meta)
}
