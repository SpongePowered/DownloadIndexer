package indexer

import (
	"crypto/md5"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"github.com/Minecrell/SpongeDownloads/db"
	"github.com/Minecrell/SpongeDownloads/downloads"
	"github.com/Minecrell/SpongeDownloads/indexer/mcmod"
	"strings"
	"time"
)

const (
	remoteSeparator    = '/'
	remoteOriginPrefix = "origin" + string(remoteSeparator)
	buildTypeSeparator = '-'

	emptyChangelog   = "[]"
	recommendedLabel = "recommended"
)

var nullTime = time.Time{}

func (s *session) createDownload(i *Indexer, displayVersion string, mainJar []byte,
	buildType, branch string, metadataBytes []byte, publishedOverride time.Time, recommended bool) error {

	manifest, published, metadata, err := readJar(mainJar, s.project.pluginID != "")
	if err != nil {
		return downloads.BadRequest("Failed to read JAR file", err)
	}
	if manifest == nil {
		return downloads.BadRequest("Missing manifest in JAR", nil)
	}

	if publishedOverride != nullTime {
		published = publishedOverride
	} else if published == nullTime {
		return downloads.BadRequest("Missing timestamps in JAR file", nil)
	}

	if metadataBytes != nil {
		metadata, err = mcmod.ReadMetadataBytes(metadataBytes)
		if err != nil {
			return downloads.BadRequest("Failed to read metadata file", err)
		}
	}

	var pluginMeta *mcmod.Metadata
	if metadata != nil {
		for _, metaEntry := range metadata {
			if metaEntry.ID == s.project.pluginID {
				pluginMeta = metaEntry
				break
			}
		}

		if pluginMeta == nil {
			return downloads.BadRequest("Missing plugin '"+s.project.pluginID+"' in "+mcmod.MetadataFileName, nil)
		} else if pluginMeta.Version != s.version {
			return downloads.BadRequest(mcmod.MetadataFileName+" version mismatch: "+s.version+" != "+pluginMeta.Version, nil)
		}
	} else if s.project.pluginID != "" {
		return downloads.BadRequest("Missing "+mcmod.MetadataFileName+" in JAR", nil)
	}

	commit := manifest["Git-Commit"]
	if commit == "" {
		return downloads.BadRequest("Missing Git-Commit in manifest", err)
	}

	if branch == "" {
		branch = manifest["Git-Branch"]
		if branch == "" {
			return downloads.BadRequest("Missing Git-Branch in manifest", err)
		}
		if strings.HasPrefix(branch, remoteOriginPrefix) {
			branch = branch[len(remoteOriginPrefix):]
		}
	}
	if strings.IndexByte(branch, remoteSeparator) != -1 {
		return downloads.BadRequest("Branch should not contain a slash", nil)
	}

	if buildType == "" {
		buildType = substringBefore(branch, buildTypeSeparator)
	}

	var buildTypeId int
	var allowsPromotion bool
	err = i.DB.QueryRow("SELECT build_type_id, allows_promotion FROM build_types "+
		"JOIN project_build_types USING(build_type_id) "+
		"WHERE project_id = $1 AND name = $2;",
		s.project.id, buildType).Scan(&buildTypeId, &allowsPromotion)
	if err != nil {
		if err == sql.ErrNoRows {
			return downloads.BadRequest("Unknown build type", err)
		} else {
			return downloads.InternalError("Database error (failed to lookup build type)", err)
		}
	}

	if recommended && !allowsPromotion {
		return downloads.BadRequest("Build type does not allow promotion", err)
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
		s.tx.QueryRow("SELECT commit FROM downloads "+
			"WHERE project_id = $1 AND build_type_id = $2 ORDER BY published DESC LIMIT 1;",
			s.project.id, buildTypeId).Scan(&parentCommit)

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
		s.project.id, buildTypeId, displayVersion, db.ToNullString(mavenVersion), published, branch, commit,
		db.ToNullString(label), db.ToNullString(changelog)).Scan(&s.downloadID)
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
	if commit == parentCommit {
		// No changes
		return emptyChangelog, nil
	}

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
