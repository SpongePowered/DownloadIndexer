package indexer

import (
	"bytes"
	"database/sql"
	"github.com/SpongePowered/SpongeDownloads/downloads"
	"github.com/SpongePowered/SpongeDownloads/git"
	"github.com/SpongePowered/SpongeDownloads/httperror"
	"github.com/SpongePowered/SpongeDownloads/maven"
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

type metaState int

const (
	maxFileSize = 64 * 1024 * 1024 // 64 MB

	jarExtension = "jar"

	sessionCookieName   = "IndexerSession"
	sessionSecretLength = 32
	sessionTimeout      = 5 * time.Minute

	metaPending metaState = iota
	metaLocked
	metaDone
)

var semVerPattern = regexp.MustCompile(
	`(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)(?:-[\dA-Za-z-]+(?:\.[\dA-Za-z-]+)*)?(?:\+[\dA-Za-z-]+(?:\.[\dA-Za-z-]+)*)?(?:-SNAPSHOT)?`)

type Indexer struct {
	*downloads.Module

	repo maven.Repository
	git  *git.Manager

	projects    map[maven.Identifier]*project
	sessions    map[string]*session
	sessionLock sync.RWMutex
}

type project struct {
	id int

	pluginID string

	githubOwner string
	githubRepo  string

	useSnapshots bool
	useSemVer    bool

	lock sync.Mutex
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

	lockedProject bool
	projectMeta   metaState
	versionMeta   metaState

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
		project := new(project)

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
	if p.version == "" {
		s, err = i.requireSession(ctx, project, p.version)
	} else if !p.snapshot {
		return httperror.BadRequest("Expected artifact upload", nil)
	} else if project.useSnapshots {
		s, err = i.getOrCreateSession(ctx, project, p.version)
	} else {
		return httperror.Forbidden("Project does not allow snapshots")
	}

	if err != nil {
		return err
	}

	defer s.unlock()

	if s.failed {
		return httperror.New(http.StatusFailedDependency, "Previous request failed", nil)
	}

	var meta *metaState

	if p.version == "" {
		meta = &s.projectMeta
	} else {
		meta = &s.versionMeta
	}

	switch *meta {
	case metaLocked:
		return httperror.New(http.StatusLocked, "Metadata is locked", nil)
	case metaDone:
		return httperror.BadRequest("Metadata was already uploaded", nil)
	}

	*meta = metaLocked
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

		// Version metadata is not needed for release versions
		s.versionMeta = metaDone
	}

	defer s.unlock()

	if s.failed {
		return httperror.New(http.StatusFailedDependency, "Previous request failed", nil)
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
				var branch string
				var metadataBytes []byte
				var published time.Time
				requireChangelog := true

				if macaron.Env == macaron.DEV {
					branch = ctx.Query("branch")

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

				err = s.createDownload(i, p.displayVersion, data, branch, metadataBytes, published,
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
		var meta *metaState

		if p.version == "" {
			meta = &s.projectMeta
		} else {
			meta = &s.versionMeta
		}

		if *meta != metaLocked {
			return httperror.Forbidden("Cannot modify metadata without a lock")
		}

		*meta = metaDone

		if s.projectMeta == metaDone && s.versionMeta == metaDone && s.tx != nil {
			// Woo, we're done!
			err = s.tx.Commit()
			if err != nil {
				return err
			}

			s.tx = nil
			s.release(i)

			go func() {
				// Attempt to update project timestamp
				_, err := i.DB.Exec("UPDATE projects SET last_updated = current_timestamp WHERE project_id = $1;", s.project.id)
				if err != nil {
					i.Log.Println("Failed to update project timestamp:", err)
				}
			}()

			if i.Cache != nil {
				go i.Cache.PurgeProject(p.Identifier)
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

func (i *Indexer) registerSession(s *session) {
	i.sessionLock.Lock()
	defer i.sessionLock.Unlock()
	i.sessions[s.id] = s
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

	// Start goroutine which will destroy the session after timeout
	go s.destroy(i)

	i.registerSession(s)

	// Lock project
	project.lock.Lock()
	s.lockedProject = true
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

	// Unlock project
	if s.lockedProject {
		s.project.lock.Unlock()
		s.lockedProject = false
	}
}

func (s *session) fail(i *Indexer) {
	if !s.failed {
		s.failed = true
		s.release(i)
	}
}

func (s *session) destroy(i *Indexer) {
	<-s.timeout.C

	s.release(i)

	i.sessionLock.Lock()
	defer i.sessionLock.Unlock()
	delete(i.sessions, s.id)
}

func decodeHash(data []byte) string {
	return strings.ToLower(strings.TrimSpace(string(data)))
}
