package db

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"code.dumpstack.io/tools/out-of-tree/api"
)

func TestJobTable(t *testing.T) {
	file, db := tmpdb(t)
	defer os.Remove(file.Name())
	defer db.Close()

	job := api.Job{
		RepoName: "testname",
		Commit:   "test",
		Group:    uuid.New().String(),
	}

	err := AddJob(db, &job)
	assert.Nil(t, err)

	job.Group = uuid.New().String()

	err = UpdateJob(db, &job)
	assert.Nil(t, err)

	jobs, err := Jobs(db)
	assert.Nil(t, err)

	assert.Equal(t, 1, len(jobs))

	assert.Equal(t, job.Group, jobs[0].Group)
}
