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
	"reflect"
	"strings"
	"sync"
	"time"
)

const (
	maxFileSize = 64 * 1024 * 1024 // 64 MB

	jarExtension = "jar"

	sessionCookieName   = "IndexerSession"
	sessionSecretLength = 32
	sessionTimeout      = 5 * time.Minute
)

type Indexer struct {
	*downloads.Service

	repo maven.Repository
	git  *git.Manager

	projects    map[maven.Identifier]*project
	sessions    map[string]*session
	sessionLock sync.RWMutex
}

type metadata struct {
	session string
	lock    sync.Mutex
}

type project struct {
	id uint

	githubOwner string
	githubRepo  string

	metadata            *metadata
	versionMetadata     map[string]*metadata
	versionMetadataLock sync.RWMutex
}

type session struct {
	id string

	project *project
	version string

	tx *sql.Tx

	downloadID uint
	artifacts  map[artifactType]*artifact

	failed  bool
	timeout *time.Timer

	lockedProjectMeta bool
	lockedVersionMeta *metadata

	_lock sync.Mutex
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

func (i *Indexer) LoadProjects() error {
	rows, err := i.DB.Query("SELECT id, group_id, artifact_id, github_owner, github_repo FROM projects;")
	if err != nil {
		return err
	}

	for rows.Next() {
		var identifier maven.Identifier
		project := &project{metadata: new(metadata), versionMetadata: make(map[string]*metadata)}

		err = rows.Scan(&project.id, &identifier.GroupID, &identifier.ArtifactID,
			&project.githubOwner, &project.githubRepo)
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

	var s *session
	var meta *metadata
	if p.version == "" {
		s, err = i.requireSession(ctx, project, p.version)
		if err != nil {
			return err
		}

		meta = project.metadata
	} else {
		s, err = i.getOrCreateSession(ctx, project, p.version)
		if err != nil {
			return err
		}

		meta = project.getOrCreateVersionMetadata(p.version)
	}

	defer s.unlock()

	if s.failed {
		return downloads.Error(http.StatusFailedDependency, "Previous request failed", nil)
	}

	if p.t == file && meta.session != s.id {
		meta.lock.Lock()
		meta.session = s.id

		if p.version == "" {
			s.lockedProjectMeta = true
		} else {
			s.lockedVersionMeta = meta
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

	defer s.unlock()

	if s.failed {
		return downloads.Error(http.StatusFailedDependency, "Previous request failed", nil)
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

		if a == nil {
			a = new(artifact)
			s.artifacts[p.artifact] = a
		}

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

		meta.unlock()

		if p.version == "" {
			s.lockedProjectMeta = false
		} else {
			s.lockedVersionMeta = nil
		}

		if !s.lockedProjectMeta && s.lockedVersionMeta == nil {
			// Woo, we're done!
			err = s.tx.Commit()
			if err != nil {
				return err
			}

			s.tx = nil

			// We let the timeout do its work to cleanup the session
		}
	}

	return i.repo.Upload(path, bytes.NewReader(data))
}

func (i *Indexer) ErrorHandler(ctx *macaron.Context) {
	ctx.Next()

	if ctx.Resp.Status() != 200 {
		// Error occurred
		sv := ctx.GetVal(reflect.TypeOf((*session)(nil)))
		if sv.IsValid() {
			s := sv.Interface().(*session)
			s.fail(i)
		}
	}
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

	s.lock(ctx)
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
		artifacts: make(map[artifactType]*artifact),
		timeout:   time.NewTimer(0),
	}

	<-s.timeout.C

	s.lock(ctx)

	i.sessions[sessionID] = s

	// Start goroutine which will delete the session after timeout
	go s.delete(i)

	return s, nil
}

func (s *session) lock(ctx *macaron.Context) {
	ctx.Map(s)

	s._lock.Lock()
	s.timeout.Stop()
}

func (s *session) unlock() {
	s.timeout.Reset(sessionTimeout)
	s._lock.Unlock()
}

func (s *session) release(i *Indexer) {
	if s.tx != nil {
		err := s.tx.Rollback()
		if err != nil {
			i.Log.Println("Failed to rollback transaction", err)
		}

		s.tx = nil
	}

	// Release metadata locks
	if s.lockedProjectMeta {
		s.project.metadata.unlock()
		s.lockedProjectMeta = false
	}

	if s.lockedVersionMeta != nil {
		s.lockedVersionMeta.unlock()
		s.lockedVersionMeta = nil
	}
}

func (s *session) fail(i *Indexer) {
	if !s.failed {
		s.failed = true
		s.release(i)
	}
}

func (s *session) delete(i *Indexer) {
	<-s.timeout.C

	s.release(i)

	i.sessionLock.Lock()
	defer i.sessionLock.Unlock()
	delete(i.sessions, s.id)
}

func (m *metadata) unlock() {
	m.session = ""
	m.lock.Unlock()
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

		m = new(metadata)
		p.versionMetadata[version] = m
	}

	return
}

func decodeHash(data []byte) string {
	return strings.ToLower(strings.TrimSpace(string(data)))
}
