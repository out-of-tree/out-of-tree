package db

import (
	"database/sql"
	"encoding/json"

	"code.dumpstack.io/tools/out-of-tree/api"
)

func createJobTable(db *sql.DB) (err error) {
	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS job (
		id		INTEGER PRIMARY KEY,
		uuid		TEXT,
		group_uuid	TEXT,
		repo		TEXT,
		"commit"	TEXT,
		config		TEXT,
		target		TEXT,
		status		TEXT DEFAULT "new"
	)`)
	return
}

func AddJob(db *sql.DB, job *api.Job) (err error) {
	stmt, err := db.Prepare(`INSERT INTO job (uuid, group_uuid, repo, "commit", ` +
		`config, target) ` +
		`VALUES ($1, $2, $3, $4, $5, $6);`)
	if err != nil {
		return
	}

	defer stmt.Close()

	config := api.Marshal(job.Artifact)
	target := api.Marshal(job.Target)

	res, err := stmt.Exec(job.UUID, job.Group,
		job.RepoName, job.Commit,
		config, target,
	)
	if err != nil {
		return
	}

	job.ID, err = res.LastInsertId()
	return
}

func UpdateJob(db *sql.DB, job *api.Job) (err error) {
	stmt, err := db.Prepare(`UPDATE job SET uuid=$1, group_uuid=$2, repo=$3, ` +
		`"commit"=$4, config=$5, target=$6, status=$7 WHERE id=$8`)
	if err != nil {
		return
	}
	defer stmt.Close()

	config := api.Marshal(job.Artifact)
	target := api.Marshal(job.Target)

	_, err = stmt.Exec(job.UUID, job.Group,
		job.RepoName, job.Commit,
		config, target,
		job.Status, job.ID)
	return
}

func Jobs(db *sql.DB) (jobs []api.Job, err error) {
	stmt, err := db.Prepare(`SELECT id, uuid, group_uuid, repo, "commit", ` +
		`config, target, status FROM job`)
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
		var job api.Job
		var config, target []byte
		err = rows.Scan(&job.ID, &job.UUID, &job.Group,
			&job.RepoName, &job.Commit,
			&config, &target, &job.Status)
		if err != nil {
			return
		}

		err = json.Unmarshal(config, &job.Artifact)
		if err != nil {
			return
		}

		err = json.Unmarshal(target, &job.Target)
		if err != nil {
			return
		}

		jobs = append(jobs, job)
	}

	return
}

func Job(db *sql.DB, uuid string) (job api.Job, err error) {
	stmt, err := db.Prepare(`SELECT id, uuid, group_uuid, ` +
		`repo, "commit", ` +
		`config, target, status ` +
		`FROM job WHERE uuid=$1`)
	if err != nil {
		return
	}
	defer stmt.Close()

	err = stmt.QueryRow(uuid).Scan(&job.ID, &job.UUID,
		&job.Group, &job.RepoName, &job.Commit,
		&job.Artifact, &job.Target, &job.Status)
	if err != nil {
		return
	}

	return
}

func JobStatus(db *sql.DB, uuid string) (st api.Status, err error) {
	stmt, err := db.Prepare(`SELECT status FROM job ` +
		`WHERE uuid=$1`)
	if err != nil {
		return
	}
	defer stmt.Close()

	err = stmt.QueryRow(uuid).Scan(&st)
	if err != nil {
		return
	}

	return
}
