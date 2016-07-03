package indexer

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"github.com/Minecrell/SpongeDownloads/downloads"
	"github.com/Minecrell/SpongeDownloads/downloads/db"
	"io"
	"strings"
	"sync"
	"time"
)

type Indexer struct {
	*downloads.Service

	cache     map[version]*download
	cacheLock sync.RWMutex
}

type version struct {
	groupID         string
	artifactID      string
	version         string
	snapshotVersion sql.NullString
}

type download struct {
	projectID uint
	lock      sync.Mutex

	id        int
	uploaded  time.Time
	artifacts map[artifactType]*artifact
}

type artifactType struct {
	classifier sql.NullString
	extension  string
}

type artifact struct {
	uploaded bool
	id       int
	size     int
	md5      sql.NullString
	sha1     sql.NullString
}

func Create(m *downloads.Manager) *Indexer {
	return &Indexer{Service: m.Service("Indexer"), cache: make(map[version]*download)}
}

// TODO: Better error handling
func (i *Indexer) Upload(path string, data []byte) (err error) {
	if strings.Contains(path, "maven-metadata.xml") || strings.Contains(path, "pom") {
		return
	}

	md5 := strings.HasSuffix(path, ".md5")
	sha1 := false

	if md5 {
		path = path[:len(path)-4] // Strip .md5 from path
	} else {
		sha1 = strings.HasSuffix(path, ".sha1")
		if sha1 {
			path = path[:len(path)-5] // Strip .sha1 from path
		}
	}

	v, at, err := parsePath(path)
	if err != nil {
		return downloads.BadRequest("Invalid path", err)
	}

	i.cacheLock.RLock()
	d := i.cache[v]
	i.cacheLock.RUnlock()

	if d == nil {
		i.cacheLock.Lock()
		d = new(download)
		i.cache[v] = d
		i.cacheLock.Unlock()

		d.lock.Lock()
		defer d.lock.Unlock()

		row := i.DB.QueryRow("SELECT id FROM projects WHERE group_id = $1 AND artifact_id = $2;", v.groupID, v.artifactID)
		err = row.Scan(&d.projectID)
		if err != nil && err != sql.ErrNoRows {
			return downloads.InternalError("Database error (failed to lookup project)", err)
		}

		if d.projectID == 0 {
			return nil
		}

		d.uploaded = time.Now()
		d.artifacts = make(map[artifactType]*artifact)
	} else {
		if d.projectID == 0 {
			return
		}

		d.lock.Lock()
		defer d.lock.Unlock()
	}

	a := d.artifacts[at]

	if a == nil {
		a = new(artifact)
		d.artifacts[at] = a
	}

	if md5 {
		err = a.setMD5(i, data)
		if err != nil {
			return
		}
	} else if sha1 {
		err = a.setSHA1(i, data)
		if err != nil {
			return
		}
	} else if !a.uploaded {
		// Is this the main artifact?
		if !at.classifier.Valid && at.extension == "jar" {
			err = d.create(i, v, data)
			if err != nil {
				return
			}

			for at, a := range d.artifacts {
				if a.uploaded && a.id == 0 {
					err = a.create(i, d, at)
					if err != nil {
						return
					}
				}
			}
		}

		a.size = len(data)
		a.uploaded = true

		if d.id > 0 {
			err = a.create(i, d, at)
			if err != nil {
				return
			}
		}
	}

	return
}

func (d *download) create(i *Indexer, v version, mainJar []byte) error {
	m, err := d.readManifest(mainJar)
	if err != nil {
		return downloads.BadRequest("Failed to read JAR file", err)
	}

	if m == nil {
		return downloads.BadRequest("Missing manifest in JAR", err)
	}

	commit := m["Git-Commit"]
	if commit == "" {
		return downloads.BadRequest("Missing Git-Commit in manifest", err)
	}

	branch := m["Git-Branch"]
	if branch == "" {
		return downloads.BadRequest("Missing Git-Branch in manifest", err)
	}

	minecraft := db.ToNullString(m["Minecraft-Version"])

	var branchID int
	row := i.DB.QueryRow("SELECT id FROM branches WHERE project_id = $1 AND name = $2;", d.projectID, branch)
	err = row.Scan(&branchID)

	var parentCommit sql.NullString
	if err != nil {
		if err != sql.ErrNoRows {
			return downloads.InternalError("Database error (failed to select branch)", err)
		}

		t, ok := substringBefore(branch, '-')
		row = i.DB.QueryRow("INSERT INTO branches VALUES (DEFAULT, $1, $2, $3, $4, FALSE) RETURNING id;", d.projectID, branch, t, !ok)
		err = row.Scan(&branchID)
		if err != nil {
			return downloads.InternalError("Database error (failed to add branch)", err)
		}
	} else {
		// Attempt to find parent commit
		row = i.DB.QueryRow("SELECT commit FROM downloads WHERE branch_id = $1 ORDER BY published DESC LIMIT 1;", branchID)
		err = row.Scan(&parentCommit)
		if err != nil && err != sql.ErrNoRows {
			return downloads.InternalError("Database error (failed to find parent commit)", err)
		}
	}

	row = i.DB.QueryRow("INSERT INTO downloads VALUES (DEFAULT, $1, $2, $3, $4, $5, $6, $7, NULL, $8) RETURNING id;",
		d.projectID, branchID, v.version, v.snapshotVersion, d.uploaded, commit, minecraft, parentCommit)
	err = row.Scan(&d.id)
	if err != nil {
		return downloads.InternalError("Database error (failed to add download)", err)
	}

	return nil
}

func (a *artifact) create(i *Indexer, d *download, at artifactType) error {
	row := i.DB.QueryRow("INSERT INTO artifacts VALUES (DEFAULT, $1, $2, $3, $4, $5, $6) RETURNING id;", d.id, at.classifier, at.extension, a.size, a.sha1, a.md5)
	if err := row.Scan(&a.id); err != nil {
		return downloads.InternalError("Database error (failed to create artifact)", err)
	}
	return nil
}

func decodeHash(data []byte) string {
	return strings.ToLower(strings.TrimSpace(string(data)))
}

func (a *artifact) setMD5(i *Indexer, data []byte) error {
	hash := decodeHash(data)

	// If artifact was already created
	if a.id > 0 {
		if _, err := i.DB.Exec("UPDATE artifacts SET md5 = $1 WHERE id = $2;", hash, a.id); err != nil {
			return downloads.InternalError("Database error (failed to update MD5 sum)", err)
		}
	} else {
		a.md5 = db.ToNullString(hash)
	}

	return nil
}

func (a *artifact) setSHA1(i *Indexer, data []byte) error {
	hash := decodeHash(data)

	// If artifact was already created
	if a.id > 0 {
		if _, err := i.DB.Exec("UPDATE artifacts SET sha1 = $1 WHERE id = $2;", hash, a.id); err != nil {
			return downloads.InternalError("Database error (failed to update SHA1 sum)", err)
		}
	} else {
		a.sha1 = db.ToNullString(hash)
	}

	return nil
}

func (d *download) readManifest(mainJar []byte) (m manifest, err error) {
	reader, err := zip.NewReader(bytes.NewReader(mainJar), int64(len(mainJar)))
	if err != nil {
		return
	}

	for _, file := range reader.File {
		if file.Name == manifestPath {
			var r io.ReadCloser
			r, err = file.Open()
			if err != nil {
				return
			}

			m, err = readManifest(r)
			return
		}
	}

	return
}

func substringBefore(s string, c byte) (string, bool) {
	i := strings.IndexByte(s, c)
	if i == -1 {
		return s, false
	}
	return s[:i], true
}
