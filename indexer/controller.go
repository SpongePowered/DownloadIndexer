package indexer

import (
	"bytes"
	"database/sql"
	"github.com/Minecrell/SpongeDownloads/downloads"
	"github.com/Minecrell/SpongeDownloads/git"
	"github.com/Minecrell/SpongeDownloads/httperror"
	"github.com/Minecrell/SpongeDownloads/maven"
	"github.com/Unknwon/com"
	"gopkg.in/macaron.v1"
	"io"
	"net/http"
	"reflect"
	"regexp"
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

	httpStatusFailedDependency = 424
)

var semVerPattern = regexp.MustCompile(
	`(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)(?:-[\dA-Za-z-]+(?:\.[\dA-Za-z-]+)*)?(?:\+[\dA-Za-z-]+(?:\.[\dA-Za-z-]+)*)?(-SNAPSHOT)?`)

type Indexer struct {
	*downloads.Module

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
	id int

	pluginID string

	githubOwner string
	githubRepo  string

	useSnapshots bool
	useSemVer    bool

	metadata            *metadata
	versionMetadata     map[string]*metadata
	versionMetadataLock sync.RWMutex
}

type session struct {
	id string

	project *project
	version string

	tx *sql.Tx

	downloadID int
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
		Module:   m.Module("Indexer"),
		repo:     repo,
		git:      git,
		projects: make(map[maven.Identifier]*project),
		sessions: make(map[string]*session),
	}
}

func (i *Indexer) LoadProjects() error {
	i.Log.Println("Loading projects")

	rows, err := i.DB.Query("SELECT project_id, group_id, artifact_id, plugin_id, github_owner, github_repo, " +
		"use_snapshots, use_semver FROM projects;")
	if err != nil {
		return err
	}

	for rows.Next() {
		var identifier maven.Identifier
		project := &project{metadata: new(metadata), versionMetadata: make(map[string]*metadata)}

		err = rows.Scan(&project.id, &identifier.GroupID, &identifier.ArtifactID, &project.pluginID,
			&project.githubOwner, &project.githubRepo, &project.useSnapshots, &project.useSemVer)
		if err != nil {
			return err
		}

		i.projects[identifier] = project
	}

	return nil
}

func (i *Indexer) Setup(m *macaron.Macaron, auth macaron.Handler) {
	m.Group("/maven/upload", func() {
		m.Get("/*", i.Get)
		m.Put("/*", i.Put)
	},
		i.InitializeContext,
		i.ErrorHandler,
		macaron.Recovery(),
		auth)
}

func (i *Indexer) Get(ctx *macaron.Context) error {
	path := ctx.Params("*")

	p, err := parsePath(path, false)
	if err != nil {
		return httperror.BadRequest("Invalid path", err)
	}

	// The uploader should only download the metadata from the indexer
	if !p.metadata {
		return httperror.Forbidden("Can only download maven metadata")
	}

	project := i.projects[p.Identifier]
	if project == nil {
		return i.repo.Download(path, ctx.Resp)
	}

	var s *session
	var meta *metadata
	if p.version == "" {
		s, err = i.requireSession(ctx, project, p.version)
		if err != nil {
			return err
		}

		meta = project.metadata
	} else if project.useSnapshots {
		s, err = i.getOrCreateSession(ctx, project, p.version)
		if err != nil {
			return err
		}

		meta = project.getOrCreateVersionMetadata(p.version)
	} else {
		return httperror.BadRequest("Project does not use snapshots", nil)
	}

	defer s.unlock()

	if s.failed {
		return httperror.New(httpStatusFailedDependency, "Previous request failed", nil)
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
		return httperror.New(http.StatusLengthRequired, "Missing content length", nil)
	}

	if ctx.Req.ContentLength > maxFileSize {
		return httperror.BadRequest("File exceeds maximal file size", nil)
	}

	path := ctx.Params("*")

	p, err := parsePath(path, true)
	if err != nil {
		return httperror.BadRequest("Invalid path", err)
	}

	project := i.projects[p.Identifier]
	if project == nil {
		return i.repo.Upload(path, ctx.Req.Body().ReadCloser())
	}

	if p.snapshot && !project.useSnapshots {
		return httperror.BadRequest("Project does not use snapshots", nil)
	}

	main := p.artifact.classifier == "" && p.artifact.extension == jarExtension

	var s *session
	if p.metadata || p.snapshot || !main {
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
		return httperror.New(httpStatusFailedDependency, "Previous request failed", nil)
	}

	// Read file from request body (with the specified length)
	data := make([]byte, ctx.Req.ContentLength)
	_, err = io.ReadFull(ctx.Req.Body().ReadCloser(), data)
	if err != nil {
		return httperror.BadRequest("Failed to read input", err)
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
				return httperror.New(http.StatusConflict, "Artifact already uploaded", nil)
			}

			if main {
				// Override query params (e.g. to index older builds)
				var buildType, branch string
				var metadataBytes []byte
				var published time.Time
				requireChangelog := true

				if macaron.Env == macaron.DEV {
					buildType, branch = ctx.Query("type"), ctx.Query("branch")

					if metadataSize := ctx.QueryInt("mcmodMetadataSize"); metadataSize > 0 {
						metadataBytes = data[:metadataSize]
						data = data[metadataSize:]
					}

					if publishedString := ctx.Query("published"); publishedString != "" {
						published, err = time.Parse(time.RFC3339, publishedString)
						if err != nil {
							return httperror.BadRequest("Failed to parse published date", err)
						}
					}

					if _, ok := ctx.Req.Form["requireChangelog"]; ok && !ctx.QueryBool("requireChangelog") {
						requireChangelog = false
					}
				}

				err = s.createDownload(i, p.displayVersion, data, buildType, branch, metadataBytes, published,
					project.useSnapshots && !p.snapshot, requireChangelog)
				if err != nil {
					return err
				}
			} else if s.downloadID == 0 {
				return httperror.BadRequest("Must upload main artifact first", nil)
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
			return httperror.Forbidden("Cannot modify metadata without a lock")
		}

		meta.unlock()

		if p.version == "" {
			s.lockedProjectMeta = false
		} else {
			s.lockedVersionMeta = nil
		}

		if !s.lockedProjectMeta && s.lockedVersionMeta == nil && s.tx != nil {
			// Woo, we're done!
			err = s.tx.Commit()
			if err != nil {
				return err
			}

			s.tx = nil

			go func() {
				// Attempt to update project timestamp
				_, err := i.DB.Exec("UPDATE projects SET last_updated = current_timestamp WHERE project_id = $1;", s.project.id)
				if err != nil {
					i.Log.Println("Failed to update project timestamp:", err)
				}
			}()

			if i.Cache != nil {
				go func() {
					// Purge cache
					err := i.Cache.PurgeProject(p.Identifier)
					if err != nil {
						i.Log.Println("Failed to purge project cache:", err)
					} else {
						i.Log.Println("Successfully purged cache for project", p.Identifier)
					}
				}()
			}

			// We let the timeout do its work to cleanup the session
		}
	}

	return i.repo.Upload(path, bytes.NewReader(data))
}

func (i *Indexer) ErrorHandler(ctx *macaron.Context) {
	ctx.Next()

	status := ctx.Resp.Status()
	if status != http.StatusOK && (ctx.Req.Method != http.MethodGet || status != http.StatusNotFound) {
		// Error occurred
		sv := ctx.GetVal(reflect.TypeOf((*session)(nil)))
		if sv.IsValid() {
			s := sv.Interface().(*session)
			s.fail(i)
		}
	}
}

func (i *Indexer) getSession(id string) *session {
	i.sessionLock.RLock()
	defer i.sessionLock.RUnlock()
	return i.sessions[id]
}

func (i *Indexer) requireSession(ctx *macaron.Context, project *project, version string) (*session, error) {
	id := ctx.GetCookie(sessionCookieName)
	if id == "" {
		return nil, httperror.Forbidden("Missing session")
	}

	s := i.getSession(id)
	if s == nil {
		return nil, httperror.Forbidden("Unknown session")
	}

	if s.project != project || (version != "" && s.version != version) {
		return s, httperror.BadRequest("Invalid session provided", nil)
	}

	s.lock(ctx)
	return s, nil
}

func (i *Indexer) getOrCreateSession(ctx *macaron.Context, project *project, version string) (*session, error) {
	s, err := i.requireSession(ctx, project, version)
	if s != nil {
		return s, err
	}

	if project.useSemVer && !semVerPattern.MatchString(version) {
		return nil, httperror.BadRequest("Version does not match semantic versioning specifications", nil)
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

	// Start goroutine which will delete the session after timeout
	go s.delete(i)

	i.sessionLock.Lock()
	defer i.sessionLock.Unlock()
	i.sessions[sessionID] = s

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
			i.Log.Println("Failed to rollback transaction:", err)
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
