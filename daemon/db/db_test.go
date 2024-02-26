package db

import (
	"database/sql"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func tmpdb(t *testing.T) (file *os.File, db *sql.DB) {
	file, err := os.CreateTemp("", "temp-sqlite.db")
	assert.Nil(t, err)
	// defer os.Remove(file.Name())

	db, err = OpenDatabase(file.Name())
	assert.Nil(t, err)
	// defer db.Close()

	return
}

func TestOpenDatabase(t *testing.T) {
	file, db := tmpdb(t)
	defer os.Remove(file.Name())
	db.Close()

	db, err := OpenDatabase(file.Name())
	assert.Nil(t, err)
	db.Close()
}
