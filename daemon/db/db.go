package db

import (
	"database/sql"
	"fmt"
	"strconv"

	_ "github.com/mattn/go-sqlite3"
)

// Change on ANY database update
const currentDatabaseVersion = 1

const versionField = "db_version"

func createMetadataTable(db *sql.DB) (err error) {
	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS metadata (
		id	INTEGER PRIMARY KEY,
		key	TEXT UNIQUE,
		value	TEXT
	)`)
	return
}

func metaChkValue(db *sql.DB, key string) (exist bool, err error) {
	sql := "SELECT EXISTS(SELECT id FROM metadata WHERE key = $1)"
	stmt, err := db.Prepare(sql)
	if err != nil {
		return
	}
	defer stmt.Close()

	err = stmt.QueryRow(key).Scan(&exist)
	return
}

func metaGetValue(db *sql.DB, key string) (value string, err error) {
	stmt, err := db.Prepare("SELECT value FROM metadata " +
		"WHERE key = $1")
	if err != nil {
		return
	}
	defer stmt.Close()

	err = stmt.QueryRow(key).Scan(&value)
	return
}

func metaSetValue(db *sql.DB, key, value string) (err error) {
	stmt, err := db.Prepare("INSERT OR REPLACE INTO metadata " +
		"(key, value) VALUES ($1, $2)")
	if err != nil {
		return
	}
	defer stmt.Close()

	_, err = stmt.Exec(key, value)
	return
}

func getVersion(db *sql.DB) (version int, err error) {
	s, err := metaGetValue(db, versionField)
	if err != nil {
		return
	}

	version, err = strconv.Atoi(s)
	return
}

func createSchema(db *sql.DB) (err error) {
	err = createMetadataTable(db)
	if err != nil {
		return
	}

	err = createJobTable(db)
	if err != nil {
		return
	}

	err = createRepoTable(db)
	if err != nil {
		return
	}

	return
}

func OpenDatabase(path string) (db *sql.DB, err error) {
	db, err = sql.Open("sqlite3", path)
	if err != nil {
		return
	}

	db.SetMaxOpenConns(1)

	exists, _ := metaChkValue(db, versionField)
	if !exists {
		err = createSchema(db)
		if err != nil {
			return
		}

		err = metaSetValue(db, versionField,
			strconv.Itoa(currentDatabaseVersion))
		return
	}

	version, err := getVersion(db)
	if err != nil {
		return
	}

	if version != currentDatabaseVersion {
		err = fmt.Errorf("database is not supported (%d instead of %d)",
			version, currentDatabaseVersion)
		return
	}

	return
}
