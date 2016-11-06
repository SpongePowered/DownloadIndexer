package db

import "database/sql"

func createTables(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE projects (
			project_id SERIAL PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,

			group_id TEXT NOT NULL,
			artifact_id TEXT NOT NULL,
			UNIQUE(group_id, artifact_id),

			plugin_id TEXT,

			github_owner TEXT NOT NULL,
			github_repo TEXT NOT NULL,
			UNIQUE(github_owner, github_repo),

			use_snapshots BOOLEAN NOT NULL,
			use_semver BOOLEAN NOT NULL
		);

		CREATE TABLE build_types (
			build_type_id SERIAL PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			allows_promotion BOOLEAN NOT NULL
		);

		CREATE TABLE project_build_types (
			project_id INT NOT NULL REFERENCES projects ON DELETE CASCADE ON UPDATE CASCADE,
			build_type_id INT NOT NULL REFERENCES build_types ON DELETE CASCADE ON UPDATE CASCADE,
			PRIMARY KEY(project_id, build_type_id)
		);

		CREATE TABLE downloads (
			download_id SERIAL PRIMARY KEY,
			project_id INT NOT NULL REFERENCES projects ON DELETE CASCADE ON UPDATE CASCADE,
			build_type_id INT NOT NULL REFERENCES build_types ON DELETE RESTRICT ON UPDATE CASCADE,

			version TEXT NOT NULL,
			snapshot_version TEXT,
			published TIMESTAMP(0) WITH TIME ZONE NOT NULL,

			branch TEXT NOT NULL,
			commit CHAR(40) NOT NULL,

			label TEXT,
			changelog JSONB,

			UNIQUE(project_id, version),
			UNIQUE(build_type_id, published)
		);

		CREATE TABLE dependencies (
			download_id INT NOT NULL REFERENCES downloads ON DELETE CASCADE ON UPDATE CASCADE,
			name TEXT NOT NULL,
			version TEXT NOT NULL,
			PRIMARY KEY(download_id, name)
		);

		CREATE TABLE artifacts (
			download_id INT NOT NULL REFERENCES downloads ON DELETE CASCADE ON UPDATE CASCADE,
			classifier TEXT,
			extension TEXT NOT NULL,
			PRIMARY KEY(download_id, classifier, extension),

			size INT NOT NULL,
			sha1 CHAR(40) NOT NULL,
			md5 CHAR(32) NOT NULL
		);
	`)

	return err
}

func dropTables(db *sql.DB) error {
	_, err := db.Exec("DROP TABLE IF EXISTS artifacts, dependencies, downloads, project_build_types, build_types, projects;")
	return err
}
