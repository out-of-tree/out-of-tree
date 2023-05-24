// Copyright 2019 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"database/sql"
	"fmt"
	"strconv"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/distro"
	"code.dumpstack.io/tools/out-of-tree/qemu"
)

// Change on ANY database update
const currentDatabaseVersion = 3

const versionField = "db_version"

type logEntry struct {
	ID        int
	Tag       string
	Timestamp time.Time

	qemu.System
	config.Artifact
	distro.KernelInfo
	phasesResult
}

func createLogTable(db *sql.DB) (err error) {
	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS log (
		id		INTEGER PRIMARY KEY,
		time		DATETIME DEFAULT CURRENT_TIMESTAMP,
		tag		TEXT,

		name		TEXT,
		type		TEXT,

		distro_type	TEXT,
		distro_release	TEXT,
		kernel_release	TEXT,

		internal_err	TEXT,

		build_output	TEXT,
		build_ok	BOOLEAN,

		run_output	TEXT,
		run_ok		BOOLEAN,

		test_output	TEXT,
		test_ok		BOOLEAN,

		qemu_stdout	TEXT,
		qemu_stderr	TEXT,

		kernel_panic	BOOLEAN,
		timeout_kill	BOOLEAN
	)`)
	return
}

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

func addToLog(db *sql.DB, q *qemu.System, ka config.Artifact,
	ki distro.KernelInfo, res *phasesResult, tag string) (err error) {

	stmt, err := db.Prepare("INSERT INTO log (name, type, tag, " +
		"distro_type, distro_release, kernel_release, " +
		"internal_err, " +
		"build_output, build_ok, " +
		"run_output, run_ok, " +
		"test_output, test_ok, " +
		"qemu_stdout, qemu_stderr, " +
		"kernel_panic, timeout_kill) " +
		"VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, " +
		"$10, $11, $12, $13, $14, $15, $16, $17);")
	if err != nil {
		return
	}

	defer stmt.Close()

	_, err = stmt.Exec(
		ka.Name, ka.Type, tag,
		ki.Distro.ID, ki.Distro.Release, ki.KernelRelease,
		res.InternalErrorString,
		res.Build.Output, res.Build.Ok,
		res.Run.Output, res.Run.Ok,
		res.Test.Output, res.Test.Ok,
		q.Stdout, q.Stderr,
		q.KernelPanic, q.KilledByTimeout,
	)
	if err != nil {
		return
	}

	return
}

func getAllLogs(db *sql.DB, tag string, num int) (les []logEntry, err error) {
	stmt, err := db.Prepare("SELECT id, time, name, type, tag, " +
		"distro_type, distro_release, kernel_release, " +
		"internal_err, " +
		"build_ok, run_ok, test_ok, kernel_panic, " +
		"timeout_kill FROM log ORDER BY datetime(time) DESC " +
		"LIMIT $1")
	if err != nil {
		return
	}
	defer stmt.Close()

	rows, err := stmt.Query(num)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var internalErr sql.NullString
		le := logEntry{}
		err = rows.Scan(&le.ID, &le.Timestamp,
			&le.Name, &le.Type, &le.Tag,
			&le.Distro.ID, &le.Distro.Release, &le.KernelRelease,
			&internalErr,
			&le.Build.Ok, &le.Run.Ok, &le.Test.Ok,
			&le.KernelPanic, &le.KilledByTimeout,
		)
		if err != nil {
			return
		}

		le.InternalErrorString = internalErr.String

		if tag == "" || tag == le.Tag {
			les = append(les, le)
		}
	}

	return
}

func getAllArtifactLogs(db *sql.DB, tag string, num int, ka config.Artifact) (
	les []logEntry, err error) {

	stmt, err := db.Prepare("SELECT id, time, name, type, tag, " +
		"distro_type, distro_release, kernel_release, " +
		"internal_err, " +
		"build_ok, run_ok, test_ok, kernel_panic, " +
		"timeout_kill FROM log WHERE name=$1 AND type=$2 " +
		"ORDER BY datetime(time) DESC LIMIT $3")
	if err != nil {
		return
	}
	defer stmt.Close()

	rows, err := stmt.Query(ka.Name, ka.Type, num)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var internalErr sql.NullString
		le := logEntry{}
		err = rows.Scan(&le.ID, &le.Timestamp,
			&le.Name, &le.Type, &le.Tag,
			&le.Distro.ID, &le.Distro.Release, &le.KernelRelease,
			&internalErr,
			&le.Build.Ok, &le.Run.Ok, &le.Test.Ok,
			&le.KernelPanic, &le.KilledByTimeout,
		)
		if err != nil {
			return
		}

		le.InternalErrorString = internalErr.String

		if tag == "" || tag == le.Tag {
			les = append(les, le)
		}
	}

	return
}

func getLogByID(db *sql.DB, id int) (le logEntry, err error) {
	stmt, err := db.Prepare("SELECT id, time, name, type, tag, " +
		"distro_type, distro_release, kernel_release, " +
		"internal_err, " +
		"build_ok, run_ok, test_ok, " +
		"build_output, run_output, test_output, " +
		"qemu_stdout, qemu_stderr, " +
		"kernel_panic, timeout_kill " +
		"FROM log WHERE id=$1")
	if err != nil {
		return
	}
	defer stmt.Close()

	var internalErr sql.NullString
	err = stmt.QueryRow(id).Scan(&le.ID, &le.Timestamp,
		&le.Name, &le.Type, &le.Tag,
		&le.Distro.ID, &le.Distro.Release, &le.KernelRelease,
		&internalErr,
		&le.Build.Ok, &le.Run.Ok, &le.Test.Ok,
		&le.Build.Output, &le.Run.Output, &le.Test.Output,
		&le.Stdout, &le.Stderr,
		&le.KernelPanic, &le.KilledByTimeout,
	)
	if err != nil {
		return
	}

	le.InternalErrorString = internalErr.String
	return
}

func getLastLog(db *sql.DB) (le logEntry, err error) {
	var internalErr sql.NullString
	err = db.QueryRow("SELECT MAX(id), time, name, type, tag, "+
		"distro_type, distro_release, kernel_release, "+
		"internal_err, "+
		"build_ok, run_ok, test_ok, "+
		"build_output, run_output, test_output, "+
		"qemu_stdout, qemu_stderr, "+
		"kernel_panic, timeout_kill "+
		"FROM log").Scan(&le.ID, &le.Timestamp,
		&le.Name, &le.Type, &le.Tag,
		&le.Distro.ID, &le.Distro.Release, &le.KernelRelease,
		&internalErr,
		&le.Build.Ok, &le.Run.Ok, &le.Test.Ok,
		&le.Build.Output, &le.Run.Output, &le.Test.Output,
		&le.Stdout, &le.Stderr,
		&le.KernelPanic, &le.KilledByTimeout,
	)

	if err != nil {
		return
	}

	le.InternalErrorString = internalErr.String
	return
}

func createSchema(db *sql.DB) (err error) {
	err = createMetadataTable(db)
	if err != nil {
		return
	}

	err = createLogTable(db)
	if err != nil {
		return
	}

	return
}

func openDatabase(path string) (db *sql.DB, err error) {
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

	if version == 1 {
		_, err = db.Exec(`ALTER TABLE log ADD tag TEXT`)
		if err != nil {
			return
		}

		err = metaSetValue(db, versionField, "2")
		if err != nil {
			return
		}

		version = 2

	} else if version == 2 {
		_, err = db.Exec(`ALTER TABLE log ADD internal_err TEXT`)
		if err != nil {
			return
		}

		err = metaSetValue(db, versionField, "3")
		if err != nil {
			return
		}

		version = 3
	}

	if version != currentDatabaseVersion {
		err = fmt.Errorf("Database is not supported (%d instead of %d)",
			version, currentDatabaseVersion)
		return
	}

	return
}
