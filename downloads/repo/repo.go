package repo

import (
	"crypto/md5"
	"encoding/hex"
	"github.com/libgit2/git2go"
	"log"
	"os"
	"path/filepath"
	"sync"
)

type Manager struct {
	Log        *log.Logger
	StorageDir string

	repos     map[string]*Repository
	reposLock sync.RWMutex
}

type Repository struct {
	*Manager
	lock sync.Mutex
	repo *git.Repository
}

func Create(log *log.Logger, dir string) (*Manager, error) {
	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		return nil, err
	}

	return &Manager{Log: log, StorageDir: dir, repos: make(map[string]*Repository)}, nil
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

		result = &Repository{Manager: m, repo: repo}
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

func (r *Repository) FetchCommit(ref string) (err error) {
	oid, err := git.NewOid(ref)
	if err != nil {
		return
	}

	return r.fetchCommit(oid)
}

func (r *Repository) fetchCommit(oid *git.Oid) (err error) {
	c, err := r.repo.LookupCommit(oid)
	if err != nil {
		r.Log.Println("Can't find commit", oid, "Fetching updates...")

		// We need to fetch first
		var remote *git.Remote
		remote, err = r.repo.Remotes.Lookup("origin")
		if err != nil {
			return
		}

		err = remote.Fetch([]string{}, &git.FetchOptions{Prune: git.FetchPruneOn}, "")
		if err != nil {
			return
		}

		c, err = r.repo.LookupCommit(oid)
		if err != nil {
			return
		}
	}

	// Update submodules
	tree, err := c.Tree()
	if err != nil {
		return
	}

	e, err := tree.EntryByPath(".gitmodules")
	if err != nil {
		// No submodules
		return nil
	}

	blob, err := r.repo.LookupBlob(e.Id)
	if err != nil {
		return
	}

	submodules := r.readSubmodules(blob.Contents())

	for path, url := range submodules {
		subCommit, err := tree.EntryByPath(path)
		if err != nil {
			r.Log.Println("Failed to lookup submodule", path, err)
			continue
		}

		r.Log.Println("Checking submodule", path, "from", url)
		subRepo, err := r.Open(url)
		if err != nil {
			r.Log.Println("Failed to open submodule", path, err)
			continue
		}

		err = subRepo.fetchCommit(subCommit.Id)
		subRepo.Close()
		if err != nil {
			r.Log.Println("Failed to check submodule", path, err)
		}
	}

	err = nil
	return
}

func (r *Repository) Close() {
	r.lock.Unlock()
}
