package git

import (
	"crypto/md5"
	"encoding/hex"
	"github.com/Minecrell/SpongeDownloads/downloads"
	"github.com/libgit2/git2go"
	"os"
	"path/filepath"
	"sync"
)

var nothing = struct{}{} // This is nothing!

type Manager struct {
	*downloads.Module
	StorageDir string

	repos     map[string]*Repository
	reposLock sync.RWMutex
}

type Repository struct {
	*Manager
	url string

	lock          sync.Mutex
	repo          *git.Repository
	failedCommits map[string]struct{}
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

		result = &Repository{Manager: m, url: url, repo: repo, failedCommits: make(map[string]struct{})}
		m.repos[url] = result
	}

	result.lock.Lock()
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

func (r *Repository) fetchIfNotFound(err error) error {
	// Look closer at the error
	gitError, ok := err.(*git.GitError)
	if !ok {
		return err
	}

	if gitError.Code == git.ErrNotFound {
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
	r.lock.Unlock()
}
