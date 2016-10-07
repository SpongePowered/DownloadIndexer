package indexer

import (
	"bytes"
	"database/sql"
	"github.com/Minecrell/SpongeDownloads/downloads"
	"github.com/Minecrell/SpongeDownloads/downloads/git"
	"github.com/Minecrell/SpongeDownloads/downloads/maven"
	"github.com/Unknwon/com"
	"gopkg.in/macaron.v1"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	maxFileSize = 64 * 1024 * 1024 // 64 MB

	projectTimeout = 5 * time.Minute
	versionTimeout = 15 * time.Minute

	jarExtension = "jar"

	sessionCookieName   = "IndexerSession"
	sessionSecretLength = 32
)

type Indexer struct {
	*downloads.Service

	repo maven.Repository
	git  *git.Manager

	projects map[maven.Identifier]*project
	sessions map[string]*session
}

type metadata struct {
	session string
	lock    sync.Mutex
	timeout *time.Timer
}

type project struct {
	metadata *metadata
	id       uint

	versionMetadata     map[string]*metadata
	versionMetadataLock sync.RWMutex
}

type session struct {
	id string

	project *project
	version string

	tx *sql.Tx

	downloadID uint
	artifacts  map[artifactType]artifact

	lock sync.Mutex

	lockedMetadata int
}

type artifactType struct {
	classifier string
	extension  string
}

type artifact struct {
	uploaded bool
	md5      string
	sha1     string
}

func Create(m *downloads.Manager, repo maven.Repository, git *git.Manager) *Indexer {
	return &Indexer{
		Service:  m.Service("Indexer"),
		repo:     repo,
		git:      git,
		projects: make(map[maven.Identifier]*project),
		sessions: make(map[string]*session),
	}
}

func createMetadata() *metadata {
	m := &metadata{timeout: time.NewTimer(0)}
	m.timeout.Stop()
	return m
}

func (i *Indexer) LoadProjects() error {
	rows, err := i.DB.Query("SELECT id, group_id, artifact_id FROM projects;")
	if err != nil {
		return err
	}

	for rows.Next() {
		var identifier maven.Identifier
		project := &project{metadata: createMetadata(), versionMetadata: make(map[string]*metadata)}

		err = rows.Scan(&project.id, &identifier.GroupID, &identifier.ArtifactID)
		if err != nil {
			return err
		}

		i.projects[identifier] = project
	}

	return nil
}

func (i *Indexer) Get(ctx *macaron.Context) error {
	path := ctx.Params("*")

	p, err := parsePath(path, false)
	if err != nil {
		return downloads.BadRequest("Invalid path", err)
	}

	// The uploader should only download the metadata from the indexer
	if !p.metadata {
		return downloads.Forbidden("Can only download maven metadata")
	}

	project, ok := i.projects[p.Identifier]
	if !ok {
		return downloads.NotFound("Unknown project")
	}

	var session *session
	var meta *metadata
	if p.version == "" {
		session, err = i.requireSession(ctx, project, p.version)
		if err != nil {
			return err
		}

		meta = project.metadata
	} else {
		session, err = i.getOrCreateSession(ctx, project, p.version)
		if err != nil {
			return err
		}

		meta = project.getOrCreateVersionMetadata(p.version)
	}

	if p.t == file && meta.session != session.id {
		meta.lock.Lock()
		session.lockedMetadata++
		meta.session = session.id

		go func() {
			<-meta.timeout.C
			meta.session = ""
			meta.lock.Unlock()
		}()

		if p.version == "" {
			meta.timeout.Reset(projectTimeout)
		} else {
			meta.timeout.Reset(versionTimeout)
		}
	}

	return i.repo.Download(path, ctx.Resp)
}

func (i *Indexer) Put(ctx *macaron.Context) error {
	if ctx.Req.ContentLength <= 0 {
		return downloads.Error(http.StatusLengthRequired, "Missing content length", nil)
	}

	if ctx.Req.ContentLength > maxFileSize {
		return downloads.BadRequest("File exceeds maximal file size", nil)
	}

	path := ctx.Params("*")

	p, err := parsePath(path, true)
	if err != nil {
		return downloads.BadRequest("Invalid path", err)
	}

	project, ok := i.projects[p.Identifier]
	if !ok {
		return downloads.NotFound("Unknown project")
	}

	main := p.artifact.classifier == "" && p.artifact.extension == jarExtension

	var s *session
	if p.metadata || p.snapshotVersion != "" || !main {
		s, err = i.requireSession(ctx, project, p.version)
		if err != nil {
			return err
		}
	} else {
		s, err = i.getOrCreateSession(ctx, project, p.version)
		if err != nil {
			return err
		}
	}

	// Read file from request body (with the specified length)
	data := make([]byte, ctx.Req.ContentLength)
	_, err = io.ReadFull(ctx.Req.Body().ReadCloser(), data)
	if err != nil {
		return downloads.BadRequest("Failed to read input", err)
	}

	// TODO: Error when file is longer than specified?

	if !p.metadata {
		a := s.artifacts[p.artifact]

		// Process artifact
		switch p.t {
		case file:
			if a.uploaded {
				return downloads.Error(http.StatusConflict, "Artifact already uploaded", nil)
			}

			if main {
				err = s.createDownload(i, p.snapshotVersion, data)
				if err != nil {
					return err
				}
			} else if s.downloadID == 0 {
				return downloads.BadRequest("Must upload main artifact first", nil)
			}

			// TODO: Currently only JARs are indexed, all other artifacts only verified
			err = a.create(s, p.artifact, data, p.artifact.extension == jarExtension)
			if err != nil {
				return err
			}
		case md5File:
			err = a.setOrVerifyMD5(decodeHash(data))
			if err != nil {
				return err
			}
		case sha1File:
			err = a.setOrVerifySHA1(decodeHash(data))
			if err != nil {
				return err
			}
		}
	} else if p.t == file {
		// Check if metadata belongs to this session
		meta := project.metadata
		if p.version != "" {
			meta = project.getVersionMetadata(p.version)
		}

		if meta == nil || meta.session != s.id {
			return downloads.Forbidden("Cannot modify metadata without a lock")
		}

		meta.timeout.Reset(0) // Stop timeout and unlock
		s.lockedMetadata--

		if s.lockedMetadata <= 0 {
			// Woo, we're done!
			err = s.tx.Commit()
			if err != nil {
				return err
			}

			s.tx = nil
			s.artifacts = nil
		}
	}

	return i.repo.Upload(path, bytes.NewReader(data))
}

func (i *Indexer) requireSession(ctx *macaron.Context, project *project, version string) (*session, error) {
	sessionID := ctx.GetCookie(sessionCookieName)
	if sessionID == "" {
		return nil, downloads.Forbidden("Missing session")
	}

	s := i.sessions[sessionID]
	if s == nil {
		return nil, downloads.Forbidden("Unknown session")
	}

	if s.project != project || (version != "" && s.version != version) {
		return s, downloads.BadRequest("Invalid session provided", nil)
	}

	return s, nil
}

func (i *Indexer) getOrCreateSession(ctx *macaron.Context, project *project, version string) (*session, error) {
	s, err := i.requireSession(ctx, project, version)
	if s != nil {
		return s, err
	}

	sessionID := string(com.RandomCreateBytes(sessionSecretLength))
	ctx.SetCookie(sessionCookieName, sessionID)

	s = &session{
		id:        sessionID,
		project:   project,
		version:   version,
		artifacts: make(map[artifactType]artifact),
	}
	i.sessions[sessionID] = s
	return s, nil
}

func (p *project) getVersionMetadata(version string) *metadata {
	p.versionMetadataLock.RLock()
	defer p.versionMetadataLock.RUnlock()
	return p.versionMetadata[version]
}

func (p *project) getOrCreateVersionMetadata(version string) (m *metadata) {
	m = p.getVersionMetadata(version)
	if m == nil {
		p.versionMetadataLock.Lock()
		defer p.versionMetadataLock.Unlock()

		m = createMetadata()
		p.versionMetadata[version] = m
	}

	return
}

func decodeHash(data []byte) string {
	return strings.ToLower(strings.TrimSpace(string(data)))
}
