package git

import (
	"github.com/libgit2/git2go"
	"strings"
)

func (r *Repository) readSubmodules(tree *git.Tree) (map[string]string, error) {
	e, err := tree.EntryByPath(".gitmodules")
	if err != nil {
		gitErr, ok := err.(*git.GitError)
		if ok && gitErr.Code == git.ErrNotFound {
			// Don't return error if no submodules exist
			return nil, nil
		}
		return nil, err
	}

	blob, err := r.repo.LookupBlob(e.Id)
	if err != nil {
		return nil, err
	}

	return r.readGitModules(blob.Contents()), nil
}

func (m *Manager) readGitModules(data []byte) map[string]string {
	result := make(map[string]string)

	skip := false
	var path string
	var url string

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		length := len(line)

		if length == 0 || line[0] == '#' || line[0] == ';' {
			continue // Skip empty lines and comments
		}

		if line[0] == '[' && line[length-1] == ']' {
			if strings.HasPrefix(line, "[submodule") {
				skip = false
				if path != "" || url != "" {
					m.Log.Println("Incomplete submodule configuration", path, url)
					path = ""
					url = ""
				}
			} else {
				skip = true
			}

			continue
		}

		if skip {
			continue
		}

		pos := strings.IndexByte(line, '=')
		if pos == -1 {
			m.Log.Println("Invalid key-value pair in submodule configuration", line)
			continue
		}

		key := strings.TrimSpace(line[:pos])
		value := strings.TrimSpace(line[pos+1:])

		switch key {
		case "path":
			path = value
		case "url":
			url = value
		default:
			continue
		}

		if path != "" && url != "" {
			result[path] = url
			path = ""
			url = ""
		}
	}

	return result
}
