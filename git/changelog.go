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

func (r *Repository) GenerateChangelog(commitHash, parentHash string) ([]*Commit, error) {
	// Have we already tried (and failed) to load this commit?
	if err, ok := r.failedCommits[commitHash]; ok {
		return nil, err
	}

	id, err := git.NewOid(commitHash)
	if err != nil {
		r.failedCommits[commitHash] = err
		return nil, err
	}

	parent, err := git.NewOid(parentHash)
	if err != nil {
		return nil, err
	}

	return r.generateChangelog(id, parent)
}

func (r *Repository) generateChangelog(id *git.Oid, parent *git.Oid) ([]*Commit, error) {
	commitHash := id.String()

	// Have we already tried (and failed) to load this commit?
	if err, ok := r.failedCommits[commitHash]; ok {
		return nil, err
	}

	// Lookup commit
	_, err := r.repo.LookupCommit(id)
	if err != nil {
		err = r.fetchIfNotFound(err)
		if err != nil {
			return nil, err
		}

		_, err = r.repo.LookupCommit(id)
		if err != nil {
			r.failedCommits[commitHash] = err
			return nil, err
		}
	}

	// Check if there is a merge base between both commits (otherwise it will go back up to initial commit)
	_, err = r.repo.MergeBase(id, parent)
	if err != nil {
		return nil, err
	}

	w, err := r.repo.Walk()
	if err != nil {
		return nil, err
	}

	w.Sorting(git.SortTopological)

	err = w.Push(id)
	if err != nil {
		return nil, err
	}

	if parent != nil {
		err = w.Hide(parent)
		if err != nil {
			return nil, err
		}
	}

	var commits []*Commit

	err = w.Iterate(func(commit *git.Commit) bool {
		commits = append(commits, r.prepareCommit(commit))
		return true
	})

	return commits, nil
}

func (r *Repository) prepareCommit(commit *git.Commit) *Commit {
	author := commit.Author()
	result := &Commit{
		ID:     commit.Id().String(),
		Author: author.Name,
		Date:   author.When,
	}

	result.Title, result.Description = splitCommitMessage(commit.Message())

	// Can only generate submodule changelog for normal commits (skip initial/merge commits)
	if commit.ParentCount() == 1 {
		var err error
		result.Submodules, err = r.generateSubmoduleChangelog(commit)
		if err != nil {
			r.Log.Println("Failed to generate submodule changelog for", commit.Id(), err)
		}
	}

	return result
}

func (r *Repository) generateSubmoduleChangelog(commit *git.Commit) (map[string][]*Commit, error) {
	if commit.ParentCount() != 1 {
		// Initial commit
		return nil, nil
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}

	submodules, err := r.readSubmodules(tree)
	if err != nil {
		return nil, err
	}

	// Skip if no submodules were found
	if len(submodules) == 0 {
		return nil, nil
	}

	parentCommit := commit.Parent(0)
	parentTree, err := parentCommit.Tree()
	if err != nil {
		return nil, newError(err, "Failed to open tree for parent commit", parentCommit)
	}

	changelog := make(map[string][]*Commit)

	for path, url := range submodules {
		subEntry, err := tree.EntryByPath(path)
		if err != nil {
			r.Log.Println("Failed to get submodule ref:", err)
			continue
		}

		parentSubEntry, err := parentTree.EntryByPath(path)
		if err != nil {
			r.Log.Println("Failed to get submodule entry for parent commit:", err)
			continue
		}

		if subEntry.Id.Equal(parentSubEntry.Id) {
			// No changes
			continue
		}

		subRepo, err := r.root.open(url)
		if err != nil {
			r.Log.Println("Failed to open submodule repo:", err)
			continue
		}

		commits, err := subRepo.generateChangelog(subEntry.Id, parentSubEntry.Id)
		if err != nil {
			r.Log.Println("Failed to generate submodule changelog:", err)
			continue
		}

		if commits != nil {
			changelog[path] = commits
		}
	}

	return changelog, nil
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
