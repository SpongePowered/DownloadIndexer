package git

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"github.com/SpongePowered/SpongeDownloads/downloads"
	"gopkg.in/libgit2/git2go.v25"
	"os"
	"path/filepath"
	"sync"
)

var errRepoOpen = errors.New("Failed to open repository")

type Manager struct {
	*downloads.Module
	StorageDir string

	repos     map[string]*Repository
	reposLock sync.RWMutex
}

type Repository struct {
	*Manager
	url string

	lock sync.Mutex
	repo *git.Repository

	fetched       bool
	failedCommits map[string]error

	root     *Repository
	children map[string]*Repository
}

func Create(manager *downloads.Manager, dir string) (*Manager, error) {
	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		return nil, err
	}

	return &Manager{Module: manager.Module("Git"), StorageDir: dir, repos: make(map[string]*Repository)}, nil
}

func (m *Manager) OpenGitHub(owner, repo string) (*Repository, error) {
	return m.Open("https://github.com/" + owner + "/" + repo + ".git")
}

func (m *Manager) Open(url string) (*Repository, error) {
	m.reposLock.RLock()
	result := m.repos[url]
	m.reposLock.RUnlock()

	if result == nil {
		m.reposLock.Lock()
		defer m.reposLock.Unlock()

		repo, err := m.initRepo(url)
		if err != nil {
			return nil, err
		}

		result = &Repository{Manager: m, url: url, repo: repo, failedCommits: make(map[string]error)}
		m.repos[url] = result
	}

	result.lock.Lock()
	result.root = result
	result.children = make(map[string]*Repository)
	return result, nil
}

func (m *Manager) initRepo(url string) (*git.Repository, error) {
	// Repo dir is the MD5 hash of the repo URL
	hash := md5.Sum([]byte(url))
	dir := filepath.Join(m.StorageDir, hex.EncodeToString(hash[:]))

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		m.Log.Println("Opening", url, "from", dir)
		if repo, err := git.OpenRepository(dir); err != nil {
			m.Log.Println("Failed to open repo from:", dir, err)
			if err = os.RemoveAll(dir); err != nil {
				return nil, err
			}
		} else {
			return repo, nil
		}
	}

	m.Log.Println("Cloning", url, "to", dir)
	return git.Clone(url, dir, &git.CloneOptions{Bare: true})
}

func (r *Repository) open(url string) (*Repository, error) {
	c, ok := r.children[url]
	if !ok {
		var err error
		c, err = r.Open(url)
		if c != nil {
			c.root = r
		}
		r.children[url] = c
		return c, err
	} else if c == nil {
		return nil, errRepoOpen
	}

	return c, nil
}

func (r *Repository) fetchIfNotFound(err error) error {
	if r.fetched {
		return err
	}

	if isNotFound(err) {
		r.fetched = true

		// Try to fetch the commit
		err = r.fetch()
	}

	return err
}

func (r *Repository) fetch() (err error) {
	r.Log.Println("Fetching commits from ", r.url)

	remote, err := r.repo.Remotes.Lookup("origin")
	if err != nil {
		return
	}

	return remote.Fetch([]string{}, &git.FetchOptions{Prune: git.FetchPruneOn}, "")
}

func (r *Repository) Close() {
	r.fetched = false

	// Close all children repositories
	for _, c := range r.children {
		if c != nil {
			c.Close()
		}
	}

	r.root = nil
	r.children = nil
	r.lock.Unlock()
}

func isNotFound(err error) bool {
	gitErr, ok := err.(*git.GitError)
	return ok && gitErr.Code == git.ErrNotFound
}
