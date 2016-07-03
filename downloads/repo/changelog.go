package repo

import (
	"github.com/libgit2/git2go"
	"time"
	"strings"
)

type Commit struct {
	ID      string    `json:"id"`
	Author  string    `json:"author"`
	Date    time.Time `json:"date"`
	Message string    `json:"message"`

	Submodules map[string][]*Commit `json:"submodules,omitempty"`
}

func (r *Repository) GenerateChangelog(commitHash, parentHash string) (commits []*Commit, err error) {
	// Have we already tried (and failed) to load this commit?
	if _, ok := r.failedCommits[commitHash]; ok {
		return
	}

	id, err := git.NewOid(commitHash)
	if err != nil {
		r.failedCommits[commitHash] = nothing
		return
	}

	parent, err := git.NewOid(parentHash)
	if err != nil {
		return
	}

	return r.generateChangelog(id, []*git.Oid{parent})
}

func (r *Repository) generateChangelog(id *git.Oid, parents []*git.Oid) (commits []*Commit, err error) {
	commitHash := id.String()

	// Have we already tried (and failed) to load this commit?
	if _, ok := r.failedCommits[commitHash]; ok {
		return
	}

	w, err := r.repo.Walk()
	if err != nil {
		return
	}

	err = w.Push(id)
	if err != nil {
		err = r.fetchIfNotFound(err)
		if err != nil {
			return
		}

		err = w.Push(id)
		if err != nil {
			r.failedCommits[commitHash] = nothing
			return
		}
	}

	for _, parent := range parents {
		err = w.Hide(parent)
		if err != nil {
			r.Log.Println("Failed to push commit parent", err)
		}
	}

	err = w.Iterate(func(commit *git.Commit) bool {
		c, err := r.prepareCommit(commit)
		if err != nil {
			r.Log.Println("Failed to generate submodule changelog", err)
		}

		commits = append(commits, c)
		return true
	})
	return
}

func (r *Repository) prepareCommit(commit *git.Commit) (result *Commit, err error) {
	author := commit.Author()
	result = &Commit{
		ID:      commit.Id().String(),
		Author:  author.Name,
		Date:    author.When,
		Message: strings.TrimSpace(commit.Message()),
	}

	tree, err := commit.Tree()
	if err != nil {
		return
	}

	submodules, err := r.readSubmodules(tree)
	if err != nil {
		return
	}

	result.Submodules = make(map[string][]*Commit)

	for path, url := range submodules {
		subEntry, err := tree.EntryByPath(path)
		if err != nil {
			r.Log.Println("Failed to get submodule ref", err)
			continue
		}

		diffs := make([]*git.Oid, 0)
		for i := uint(0); i < commit.ParentCount(); i++ {
			parentCommit := commit.Parent(i)

			parentTree, err := parentCommit.Tree()
			if err != nil {
				r.Log.Println("Failed to get tree for parent commit", err)
				continue
			}

			parentSubEntry, err := parentTree.EntryByPath(path)
			if err != nil {
				r.Log.Println("Failed to get submodule entry for parent comit", err)
				continue
			}

			if !subEntry.Id.Equal(parentSubEntry.Id) {
				diffs = append(diffs, parentSubEntry.Id)
			}
		}

		if len(diffs) == 0 {
			continue
		}

		subRepo, err := r.Open(url)
		if err != nil {
			r.Log.Println("Failed to open submodule repo", err)
			continue
		}

		commits, err := subRepo.generateChangelog(subEntry.Id, diffs)
		subRepo.Close()
		if err != nil {
			r.Log.Println("Failed to generate submodule changelog", err)
		}

		result.Submodules[path] = commits
	}

	return
}
