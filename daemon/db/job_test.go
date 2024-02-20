package db

import (
	"database/sql"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"code.dumpstack.io/tools/out-of-tree/api"
)

func testCreateJobTable(t *testing.T) (file *os.File, db *sql.DB) {
	file, err := os.CreateTemp("", "temp-sqlite.db")
	assert.Nil(t, err)
	// defer os.Remove(file.Name())

	db, err = sql.Open("sqlite3", file.Name())
	assert.Nil(t, err)
	// defer db.Close()

	db.SetMaxOpenConns(1)

	err = createJobTable(db)
	assert.Nil(t, err)

	return
}

func TestJobTable(t *testing.T) {
	file, db := testCreateJobTable(t)
	defer db.Close()
	defer os.Remove(file.Name())

	job := api.Job{
		RepoName: "testname",
		Commit:   "test",
		Params:   "none",
	}

	err := AddJob(db, &job)
	assert.Nil(t, err)

	job.Params = "changed"

	err = UpdateJob(db, job)
	assert.Nil(t, err)

	jobs, err := Jobs(db)
	assert.Nil(t, err)

	assert.Equal(t, 1, len(jobs))

	assert.Equal(t, job.Params, jobs[0].Params)
}
