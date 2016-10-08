package git

import (
	"github.com/libgit2/git2go"
	"strings"
	"time"
)

const signedOff = "Signed-off-by:"

type Commit struct {
	ID          string    `json:"id"`
	Author      string    `json:"author"`
	Date        time.Time `json:"date"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`

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
		ID:     commit.Id().String(),
		Author: author.Name,
		Date:   author.When,
	}

	result.Title, result.Description = splitCommitMessage(commit.Message())

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

func splitCommitMessage(input string) (title string, message string) {
	// Remove signed off by from message (not really useful for changelog)
	i := strings.LastIndex(input, signedOff)
	if i >= 0 {
		input = input[:i]
	}

	input = strings.TrimSpace(input)

	// Attempt to normalize the line endings (convert all to just \n)
	input = normalizeLineEndings(input)

	i = strings.IndexByte(input, '\n')
	if i >= 0 {
		title = strings.TrimSpace(input[:i])
		message = strings.TrimSpace(input[i:])
	} else {
		title = input
	}

	return
}

func normalizeLineEndings(input string) string {
	input = strings.Replace(input, "\r\n", "\n", -1)
	input = strings.Replace(input, "\r", "\n", -1)
	return input
}
