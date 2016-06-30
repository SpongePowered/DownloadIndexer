package indexer

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"errors"
	"io"
	"strings"
	"sync"
	"time"
)

type Indexer struct {
	db     *sql.DB
	Target string

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

	id        uint
	uploaded  time.Time
	artifacts map[artifactType]*artifact
}

type artifactType struct {
	classifier sql.NullString
	extension  string
}

type artifact struct {
	uploaded bool
	id       uint
	md5      sql.NullString
	sha1     sql.NullString
}

func CreatePostgres(host, user, password, database string, target string) (*Indexer, error) {
	db, err := connectPostgres(host, user, password, database)
	if err != nil {
		return nil, err
	}

	// Make sure target ends with a slash
	if target[len(target)-1] != '/' {
		target += "/"
	}

	return &Indexer{db: db, Target: target, cache: make(map[version]*download)}, nil
}

func (i *Indexer) Init(create bool) (err error) {
	if create {
		err = dropTables(i.db)
		if err != nil {
			return
		}

		err = createTables(i.db)
		if err != nil {
			return
		}

		err = addProjects(i.db)
	}

	return
}

func (i *Indexer) Redirect(path string) string {
	return i.Target + path
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
		return
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

		row := i.db.QueryRow("SELECT id FROM projects WHERE group_ID = $1 AND artifact_id = $2;", v.groupID, v.artifactID)
		err = row.Scan(&d.projectID)
		if err != nil && err != sql.ErrNoRows {
			return
		}

		if d.projectID == 0 {
			return
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
		err = a.SetMD5(i, data)
	} else if sha1 {
		err = a.SetSHA1(i, data)
	} else if a.uploaded {
		err = errors.New("Artifact already uploaded")
	} else {
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

		a.uploaded = true

		if d.id > 0 {
			err = a.create(i, d, at)
		}
	}

	return
}

func (d *download) create(i *Indexer, v version, mainJar []byte) (err error) {
	m, err := d.readManifest(mainJar)
	if err != nil {
		return
	}

	if m == nil {
		err = errors.New("Missing JAR manifest")
		return
	}

	commit := m["Git-Commit"]
	if commit == "" {
		err = errors.New("Missing Git commit in JAR manifest")
		return
	}

	branch := m["Git-Branch"]
	if branch == "" {
		err = errors.New("Missing Git branch in JAR manifes")
		return
	}

	var branchID uint
	row := i.db.QueryRow("SELECT id FROM branches WHERE project_id = $1 AND name = $2;", d.projectID, branch)
	err = row.Scan(&branchID)
	if err != nil {
		if err != sql.ErrNoRows {
			return
		}

		t, ok := substringBefore(branch, '-')
		row = i.db.QueryRow("INSERT INTO branches VALUES (DEFAULT, $1, $2, $3, $4, FALSE) RETURNING id;", d.projectID, branch, t, !ok)
		err = row.Scan(&branchID)
		if err != nil {
			return
		}
	}

	row = i.db.QueryRow("INSERT INTO downloads VALUES (DEFAULT, $1, $2, NULL, $3, $4, $5) RETURNING id;", v.version, v.snapshotVersion, d.uploaded, commit, branchID)
	err = row.Scan(&d.id)
	return
}

func (a *artifact) create(i *Indexer, d *download, at artifactType) (err error) {
	row := i.db.QueryRow("INSERT INTO artifacts VALUES (DEFAULT, $1, $2, $3, $4, $5) RETURNING id;", d.id, at.classifier, at.extension, a.sha1, a.md5)
	err = row.Scan(&a.id)
	return
}

func decodeHash(data []byte) string {
	return strings.ToLower(strings.TrimSpace(string(data)))
}

func (a *artifact) SetMD5(i *Indexer, data []byte) (err error) {
	hash := decodeHash(data)

	// If artifact was already created
	if a.id > 0 {
		_, err = i.db.Exec("UPDATE artifacts SET md5 = $1 WHERE id = $2;", hash, a.id)
	} else {
		a.md5 = toNullString(hash)
	}

	return
}

func (a *artifact) SetSHA1(i *Indexer, data []byte) (err error) {
	hash := decodeHash(data)

	// If artifact was already created
	if a.id > 0 {
		_, err = i.db.Exec("UPDATE artifacts SET sha1 = $1 WHERE id = $2;", hash, a.id)
	} else {
		a.sha1 = toNullString(hash)
	}

	return
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
