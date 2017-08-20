package db

import "database/sql"

func setupProjects(db *sql.DB) error {
	stable, err := setupBuildType(db, "stable", true)
	if err != nil {
		return err
	}

	bleeding, err := setupBuildType(db, "bleeding", false)
	if err != nil {
		return err
	}

	unstable, err := setupBuildType(db, "unstable", false)
	if err != nil {
		return err
	}

	err = setupProject(db, "SpongeVanilla", "org.spongepowered", "spongevanilla", "sponge", "SpongePowered", "SpongeVanilla",
		false, false, stable, bleeding, unstable)
	if err != nil {
		return err
	}

	err = setupProject(db, "SpongeForge", "org.spongepowered", "spongeforge", "sponge", "SpongePowered", "SpongeForge",
		false, false, stable, bleeding, unstable)
	if err != nil {
		return err
	}

	err = setupProject(db, "SpongeAPI", "org.spongepowered", "spongeapi", "spongeapi", "SpongePowered", "SpongeAPI",
		true, true, stable, bleeding)
	if err != nil {
		return err
	}

	return nil
}

func setupBuildType(db *sql.DB, name string, allowsPromotion bool) (b int, err error) {
	err = db.QueryRow("INSERT INTO build_types VALUES(DEFAULT, $1, $2) RETURNING build_type_id;",
		name, allowsPromotion).Scan(&b)
	return
}

func setupProject(db *sql.DB, name, groupID, artifactID, pluginID, githubOwner, githubRepo string, snapshots bool,
	semver bool, buildTypes ...int) error {

	var projectID int
	err := db.QueryRow("INSERT INTO projects VALUES (DEFAULT, $1, $2, $3, $4, $5, $6, $7, $8) RETURNING project_id;",
		name, groupID, artifactID, ToNullString(pluginID), githubOwner, githubRepo, snapshots, semver).Scan(&projectID)
	if err != nil {
		return err
	}

	for _, buildType := range buildTypes {
		_, err = db.Exec("INSERT INTO project_build_types VALUES ($1, $2);", projectID, buildType)
		if err != nil {
			return err
		}
	}

	// Setup branch for "master" for all older builds
	_, err = db.Exec("INSERT INTO branches VALUES(DEFAULT, $1, $2, $3, DEFAULT, $4);", buildTypes[1], projectID, "master", false)
	return err
}
