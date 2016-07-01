package db

import "database/sql"

func createTables(db *sql.DB) (err error) {
	_, err = db.Exec(
		`CREATE TABLE projects (
			id serial primary key,
			identifier varchar(32) not null UNIQUE,
			name varchar(32) not null,
			url varchar(96) not null,
			group_id varchar(32) not null,
			artifact_id varchar(32) not null,
			UNIQUE(group_id, artifact_id)
		);`,
	)

	if err != nil {
		return
	}

	_, err = db.Exec(
		`CREATE TABLE branches (
			id serial primary key,
			project_id int references projects(id) not null,
			name varchar(32) not null,
			type varchar(16) not null,
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
			project_id int references projects(id) not null,
			version varchar(32) not null,
			snapshot_version varchar(32),
			minecraft varchar(16),
			label varchar(32),
			published timestamp(0) with time zone not null,
			commit char(40) not null,
			branch_id int references branches(id) not null,
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
			classifier varchar(16),
			extension varchar(8) not null,
			sha1 char(40),
			md5 char(32)
		);`,
	)

	return
}

func addProjects(db *sql.DB) (err error) {
	_, err = db.Exec(`INSERT INTO projects VALUES (
		DEFAULT,
		'maventest',
		'MavenTest',
		'https://github.com/Minecrell/maventest',
		'net.minecrell',
		'maventest'
	);`)

	return
}

func dropTables(db *sql.DB) (err error) {
	_, err = db.Exec("DROP TABLE IF EXISTS projects, branches, downloads, artifacts;")
	return
}
