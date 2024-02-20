package db

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpenDatabase(t *testing.T) {
	file, err := os.CreateTemp("", "temp-sqlite.db")
	assert.Nil(t, err)
	defer os.Remove(file.Name())

	db, err := OpenDatabase(file.Name())
	assert.Nil(t, err)
	db.Close()

	db, err = OpenDatabase(file.Name())
	assert.Nil(t, err)
	db.Close()
}
