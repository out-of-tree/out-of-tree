package db

import (
	"database/sql"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"code.dumpstack.io/tools/out-of-tree/api"
)

func testCreateRepoTable(t *testing.T) (file *os.File, db *sql.DB) {
	file, err := os.CreateTemp("", "temp-sqlite.db")
	assert.Nil(t, err)
	// defer os.Remove(tempDB.Name())

	db, err = sql.Open("sqlite3", file.Name())
	assert.Nil(t, err)
	// defer db.Close()

	db.SetMaxOpenConns(1)

	err = createRepoTable(db)
	assert.Nil(t, err)

	return
}

func TestRepoTable(t *testing.T) {
	file, db := testCreateRepoTable(t)
	defer db.Close()
	defer os.Remove(file.Name())

	repo := api.Repo{Name: "testname"}

	err := AddRepo(db, &repo)
	assert.Nil(t, err)

	repos, err := Repos(db)
	assert.Nil(t, err)

	assert.Equal(t, 1, len(repos))

	assert.Equal(t, repo, repos[0])
}
