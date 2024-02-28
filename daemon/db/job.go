package db

import (
	"bytes"
	"database/sql"
	"encoding/gob"
	"time"

	"code.dumpstack.io/tools/out-of-tree/api"
)

func createJobTable(db *sql.DB) (err error) {
	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS job (
		id		INTEGER PRIMARY KEY,
		updated		INT,
		uuid		TEXT,
		group_uuid	TEXT,
		repo		TEXT,
		"commit"	TEXT,
		config		TEXT,
		target		TEXT,
		created		INT,
		started		INT,
		finished	INT,
		status		TEXT DEFAULT "new"
	)`)
	return
}

func AddJob(db *sql.DB, job *api.Job) (err error) {
	stmt, err := db.Prepare(`INSERT INTO job (updated, uuid, group_uuid, repo, "commit", ` +
		`config, target, created, started, finished) ` +
		`VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);`)
	if err != nil {
		return
	}

	defer stmt.Close()

	var abuf bytes.Buffer
	err = gob.NewEncoder(&abuf).Encode(job.Artifact)
	if err != nil {
		return
	}
	config := abuf.Bytes()

	var tbuf bytes.Buffer
	err = gob.NewEncoder(&tbuf).Encode(job.Target)
	if err != nil {
		return
	}
	target := tbuf.Bytes()

	res, err := stmt.Exec(time.Now().Unix(), job.UUID, job.Group,
		job.RepoName, job.Commit, config, target,
		job.Created.Unix(), job.Started.Unix(),
		job.Finished.Unix(),
	)
	if err != nil {
		return
	}

	job.ID, err = res.LastInsertId()
	return
}

func UpdateJob(db *sql.DB, job *api.Job) (err error) {
	stmt, err := db.Prepare(`UPDATE job ` +
		`SET updated=$1, uuid=$2, group_uuid=$3, repo=$4, ` +
		`"commit"=$5, config=$6, target=$7, ` +
		`created=$8, started=$9, finished=$10, ` +
		`status=$11 ` +
		`WHERE id=$12`)
	if err != nil {
		return
	}
	defer stmt.Close()

	var abuf bytes.Buffer
	err = gob.NewEncoder(&abuf).Encode(job.Artifact)
	if err != nil {
		return
	}
	config := abuf.Bytes()

	var tbuf bytes.Buffer
	err = gob.NewEncoder(&tbuf).Encode(job.Target)
	if err != nil {
		return
	}
	target := tbuf.Bytes()

	_, err = stmt.Exec(time.Now().Unix(), job.UUID, job.Group,
		job.RepoName, job.Commit,
		config, target,
		job.Created.Unix(), job.Started.Unix(),
		job.Finished.Unix(), job.Status, job.ID)
	return
}

func scanJob(scan func(dest ...any) error) (job api.Job, err error) {
	var config, target []byte
	var updated, created, started, finished int64
	err = scan(&job.ID, &updated, &job.UUID, &job.Group,
		&job.RepoName, &job.Commit, &config, &target,
		&created, &started, &finished, &job.Status)
	if err != nil {
		return
	}

	abuf := bytes.NewBuffer(config)
	err = gob.NewDecoder(abuf).Decode(&job.Artifact)
	if err != nil {
		return
	}

	tbuf := bytes.NewBuffer(target)
	err = gob.NewDecoder(tbuf).Decode(&job.Target)
	if err != nil {
		return
	}

	job.UpdatedAt = time.Unix(updated, 0)
	job.Created = time.Unix(created, 0)
	job.Started = time.Unix(started, 0)
	job.Finished = time.Unix(finished, 0)
	return
}

func Jobs(db *sql.DB, where string, args ...any) (jobs []api.Job, err error) {
	q := `SELECT id, updated, uuid, group_uuid, ` +
		`repo, "commit", config, target, created, ` +
		`started, finished, status FROM job`
	if len(where) != 0 {
		q += ` WHERE ` + where
	}
	stmt, err := db.Prepare(q)
	if err != nil {
		return
	}

	defer stmt.Close()

	rows, err := stmt.Query(args...)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var job api.Job
		job, err = scanJob(rows.Scan)
		if err != nil {
			return
		}
		jobs = append(jobs, job)
	}

	return
}

func Job(db *sql.DB, uuid string) (job api.Job, err error) {
	stmt, err := db.Prepare(`SELECT id, updated, uuid, ` +
		`group_uuid, ` +
		`repo, "commit", config, target, ` +
		`created, started, finished, status ` +
		`FROM job WHERE uuid=$1`)
	if err != nil {
		return
	}
	defer stmt.Close()

	return scanJob(stmt.QueryRow(uuid).Scan)
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
