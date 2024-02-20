package db

import (
	"database/sql"

	"code.dumpstack.io/tools/out-of-tree/api"
)

func createRepoTable(db *sql.DB) (err error) {
	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS repo (
		id	INTEGER PRIMARY KEY,
		name	TEXT UNIQUE
	)`)
	return
}

func AddRepo(db *sql.DB, repo *api.Repo) (err error) {
	stmt, err := db.Prepare(`INSERT INTO repo (name) ` +
		`VALUES ($1);`)
	if err != nil {
		return
	}

	defer stmt.Close()

	res, err := stmt.Exec(repo.Name)
	if err != nil {
		return
	}

	repo.ID, err = res.LastInsertId()
	return
}

func Repos(db *sql.DB) (repos []api.Repo, err error) {
	stmt, err := db.Prepare(`SELECT id, name FROM repo`)
	if err != nil {
		return
	}

	defer stmt.Close()

	rows, err := stmt.Query()
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var repo api.Repo
		err = rows.Scan(&repo.ID, &repo.Name)
		if err != nil {
			return
		}

		repos = append(repos, repo)
	}

	return
}
