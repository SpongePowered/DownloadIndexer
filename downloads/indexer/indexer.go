package indexer

import (
	"crypto/md5"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"github.com/Minecrell/SpongeDownloads/downloads"
	"github.com/Minecrell/SpongeDownloads/downloads/db"
	"time"
)

func (s *session) createDownload(i *Indexer, snapshotVersion string, mainJar []byte) error {
	m, err := readManifestFromZip(mainJar)
	if err != nil {
		return downloads.BadRequest("Failed to read JAR file", err)
	}

	if m == nil {
		return downloads.BadRequest("Missing manifest in JAR", err)
	}

	commit := m["Git-Commit"]
	if commit == "" {
		return downloads.BadRequest("Missing Git-Commit in manifest", err)
	}

	branch := m["Git-Branch"]
	if branch == "" {
		return downloads.BadRequest("Missing Git-Branch in manifest", err)
	}

	minecraft := m["Minecraft-Version"]
	if minecraft == "" {
		return downloads.BadRequest("Missing Minecraft-Version in manifest", err)
	}

	// Start transaction
	s.tx, err = i.DB.Begin()
	if err != nil {
		return downloads.InternalError("Database error (failed to start transaction)", err)
	}

	// Run branch insert separate from transaction to prevent locks
	var branchID int
	row := i.DB.QueryRow("SELECT id FROM branches WHERE project_id = $1 AND name = $2;", s.project.id, branch)
	err = row.Scan(&branchID)

	var parentCommit sql.NullString
	if err != nil {
		if err != sql.ErrNoRows {
			return downloads.InternalError("Database error (failed to select branch)", err)
		}

		t, ok := substringBefore(branch, '-')
		row = i.DB.QueryRow("INSERT INTO branches VALUES (DEFAULT, $1, $2, $3, $4, FALSE) RETURNING id;", s.project.id, branch, t, !ok)
		err = row.Scan(&branchID)
		if err != nil {
			return downloads.InternalError("Database error (failed to add branch)", err)
		}
	} else {
		// Attempt to find parent commit
		row = s.tx.QueryRow("SELECT commit FROM downloads WHERE branch_id = $1 ORDER BY published DESC LIMIT 1;", branchID)
		err = row.Scan(&parentCommit)
		if err != nil && err != sql.ErrNoRows {
			return downloads.InternalError("Database error (failed to find parent commit)", err)
		}
	}

	var changelog string
	if parentCommit.Valid {
		changelog, err = i.generateChangelog(s.project, commit, parentCommit.String)
		if err != nil {
			return downloads.InternalError("Git error (failed to fetch commit)", err)
		}
	}

	row = s.tx.QueryRow("INSERT INTO downloads VALUES (DEFAULT, $1, $2, $3, $4, $5, $6, $7, NULL, $8) RETURNING id;",
		s.project.id, branchID, s.version, db.ToNullString(snapshotVersion), time.Now(), commit, minecraft,
		db.ToNullString(changelog))
	err = row.Scan(&s.downloadID)
	if err != nil {
		return downloads.InternalError("Database error (failed to add download)", err)
	}

	return nil
}

func (i *Indexer) generateChangelog(p *project, commit string, parentCommit string) (string, error) {
	// Generate changelog
	repo, err := i.git.OpenGitHub(p.githubOwner, p.githubRepo)
	if err != nil {
		return "", downloads.InternalError("Git error (failed to open repository)", err)
	}

	defer repo.Close()

	changelog, err := repo.GenerateChangelog(commit, parentCommit)
	if err != nil {
		return "", downloads.InternalError("Git error (failed to generate changelog)", err)
	}

	json, err := json.Marshal(changelog)
	if err != nil {
		return "", downloads.InternalError("Git error (failed to serialize changelog)", err)
	}

	return string(json), nil
}

func (a *artifact) create(s *session, t artifactType, data []byte, upload bool) (err error) {
	a.uploaded = true

	md5SumBytes := md5.Sum(data)
	md5Sum := hex.EncodeToString(md5SumBytes[:])

	err = a.setOrVerifyMD5(md5Sum)
	if err != nil {
		return
	}

	sha1SumBytes := sha1.Sum(data)
	sha1Sum := hex.EncodeToString(sha1SumBytes[:])

	err = a.setOrVerifySHA1(sha1Sum)
	if err != nil {
		return
	}

	if !upload {
		return
	}

	_, err = s.tx.Exec("INSERT INTO artifacts VALUES (DEFAULT, $1, $2, $3, $4, $5, $6);",
		s.downloadID, t.classifier, t.extension, len(data), a.sha1, a.md5)
	if err != nil {
		return downloads.InternalError("Database error (failed to create artifact)", err)
	}

	return
}

func (a *artifact) setOrVerifyMD5(md5Sum string) error {
	if a.md5 == "" {
		a.md5 = md5Sum
	} else if a.md5 != md5Sum {
		return downloads.BadRequest("MD5 checksum mismatch: "+a.md5+" != "+md5Sum, nil)
	}

	return nil
}

func (a *artifact) setOrVerifySHA1(sha1Sum string) error {
	if a.sha1 == "" {
		a.sha1 = sha1Sum
	} else if a.sha1 != sha1Sum {
		return downloads.BadRequest("SHA1 checksum mismatch: "+a.sha1+" != "+sha1Sum, nil)
	}

	return nil
}
