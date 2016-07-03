package db

import "database/sql"

func createTables(db *sql.DB) (err error) {
	_, err = db.Exec(
		`CREATE TABLE projects (
			id smallserial primary key,
			group_id text not null,
			artifact_id text not null,
			name text not null,
			github_owner text not null,
			github_repo text not null,
			UNIQUE(group_id, artifact_id),
			UNIQUE(github_owner, github_repo)
		);`,
	)

	if err != nil {
		return
	}

	_, err = db.Exec(
		`CREATE TABLE branches (
			id smallserial primary key,
			project_id smallint references projects(id) not null,
			name text not null,
			type text not null,
			main boolean not null,
			obsolete boolean not null,
			UNIQUE(project_id, name)
		);`,
	)

	if err != nil {
		return
	}

	_, err = db.Exec(
		`CREATE TABLE downloads (
			id serial primary key,
			project_id smallint references projects(id) not null,
			branch_id smallint references branches(id) not null,
			version text not null,
			snapshot_version text,
			published timestamp(0) with time zone not null,
			commit char(40) not null,
			minecraft text,
			label text,
			parent_commit char(40),
			UNIQUE(branch_id, published)
		);`,
	)

	if err != nil {
		return
	}

	_, err = db.Exec(
		`CREATE TABLE artifacts (
			id serial primary key,
			download_id int references downloads(id) not null,
			classifier text,
			extension text not null,
			size int not null,
			sha1 char(40),
			md5 char(32)
		);`,
	)

	return
}

func addProjects(db *sql.DB) (err error) {
	_, err = db.Exec(`INSERT INTO projects VALUES (
		DEFAULT,
		'net.minecrell',
		'maventest',
		'MavenTest',
		'SpongePowered',
		'SpongeVanilla'
	);`)

	return
}

func dropTables(db *sql.DB) (err error) {
	_, err = db.Exec("DROP TABLE IF EXISTS projects, branches, downloads, artifacts;")
	return
}
