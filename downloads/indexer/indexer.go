package indexer

import (
	"crypto/md5"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"github.com/Minecrell/SpongeDownloads/downloads"
	"github.com/Minecrell/SpongeDownloads/downloads/db"
	"github.com/Minecrell/SpongeDownloads/downloads/indexer/meta"
	"strings"
)

const (
	remoteSeparator    = '/'
	remoteOriginPrefix = "origin" + string(remoteSeparator)
	buildTypeSeparator = '-'

	recommendedLabel = "recommended"
)

func (s *session) createDownload(i *Indexer, displayVersion string, mainJar []byte, recommended bool) error {
	m, metadata, time, err := readJar(mainJar, s.project.pluginID != "")
	if err != nil {
		return downloads.BadRequest("Failed to read JAR file", err)
	}
	if m == nil {
		return downloads.BadRequest("Missing manifest in JAR", nil)
	}

	var pluginMeta *meta.PluginMetadata
	if metadata != nil {
		for _, metaEntry := range metadata {
			if metaEntry.ID == s.project.pluginID {
				pluginMeta = metaEntry
				break
			}
		}

		if pluginMeta == nil {
			return downloads.BadRequest("Missing plugin '"+s.project.pluginID+"' in "+meta.FileName, nil)
		} else if pluginMeta.Version != s.version {
			return downloads.BadRequest(meta.FileName+" version mismatch: "+s.version+" != "+pluginMeta.Version, nil)
		}
	} else if s.project.pluginID != "" {
		return downloads.BadRequest("Missing "+meta.FileName+" in JAR", nil)
	}

	commit := m["Git-Commit"]
	if commit == "" {
		return downloads.BadRequest("Missing Git-Commit in manifest", err)
	}

	branch := m["Git-Branch"]
	if branch == "" {
		return downloads.BadRequest("Missing Git-Branch in manifest", err)
	}
	if strings.HasPrefix(branch, remoteOriginPrefix) {
		branch = branch[len(remoteOriginPrefix):]
	}
	if strings.IndexByte(branch, remoteSeparator) != -1 {
		return downloads.BadRequest("Branch should not contain a slash", nil)
	}

	var buildTypeId int
	err = i.DB.QueryRow("SELECT build_type_id FROM build_types JOIN project_build_types USING(build_type_id)"+
		"WHERE project_id = $1 AND name = $2;",
		s.project.id, substringBefore(branch, buildTypeSeparator)).Scan(&buildTypeId)
	if err != nil {
		if err == sql.ErrNoRows {
			return downloads.BadRequest("Unknown build type", err)
		} else {
			return downloads.InternalError("Database error (failed to lookup build type)", err)
		}
	}

	// Start transaction
	s.tx, err = i.DB.Begin()
	if err != nil {
		return downloads.InternalError("Database error (failed to start transaction)", err)
	}

	var changelog string

	// Attempt to find parent commit
	if buildTypeId > 0 {
		var parentCommit string
		s.tx.QueryRow("SELECT commit FROM downloads WHERE build_type_id = $1 ORDER BY published DESC LIMIT 1;",
			buildTypeId).Scan(&parentCommit)

		if parentCommit != "" {
			// Parent commit found, generate changelog
			changelog, err = i.generateChangelog(s.project, commit, parentCommit)
			if err != nil {
				return err
			}
		}
	}

	label := ""
	if recommended {
		label = recommendedLabel
	}

	mavenVersion := s.version
	if mavenVersion == displayVersion {
		mavenVersion = ""
	}

	err = s.tx.QueryRow("INSERT INTO downloads VALUES (DEFAULT, $1, $2, $3, $4, $5, $6, $7, $8, $9) "+
		"RETURNING download_id;",
		s.project.id, buildTypeId, displayVersion, db.ToNullString(mavenVersion), time, branch, commit, db.ToNullString(label),
		db.ToNullString(changelog)).Scan(&s.downloadID)
	if err != nil {
		return downloads.InternalError("Database error (failed to add download)", err)
	}

	// Insert dependencies (if available)
	if pluginMeta != nil {
		for _, dependency := range pluginMeta.Dependencies {
			dependency.Version = cleanVersion(dependency.Version)
			if dependency.Version != "" {
				_, err = s.tx.Exec("INSERT INTO dependencies VALUES ($1, $2, $3);",
					s.downloadID, dependency.ID, dependency.Version)
				if err != nil {
					return downloads.InternalError("Database error (failed to add dependency)", err)
				}
			} else {
				i.Log.Println("Skipping dependency", dependency.ID, "(missing version)")
			}
		}
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

	jsonBytes, err := json.Marshal(changelog)
	if err != nil {
		return "", downloads.InternalError("Git error (failed to serialize changelog)", err)
	}

	return string(jsonBytes), nil
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

	_, err = s.tx.Exec("INSERT INTO artifacts VALUES ($1, $2, $3, $4, $5, $6);",
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

func cleanVersion(v string) string {
	if len(v) >= 2 && v[0] == '[' && v[len(v)-1] == ']' {
		return v[1 : len(v)-1]
	} else {
		return v
	}
}
