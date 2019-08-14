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
	"code.dumpstack.io/tools/out-of-tree/qemu"
)

const CURRENT_DATABASE_VERSION = 1

const VERSION_FIELD = "db_version"

type logEntry struct {
	ID        int
	Timestamp time.Time

	qemu.QemuSystem
	config.Artifact
	config.KernelInfo
	phasesResult
}

func createLogTable(db *sql.DB) (err error) {
	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS log (
		id		INTEGER PRIMARY KEY,
		time		DATETIME DEFAULT CURRENT_TIMESTAMP,

		name		TEXT,
		type		TEXT,

		distro_type	TEXT,
		distro_release	TEXT,
		kernel_release	TEXT,

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
		key	TEXT,
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
	stmt, err := db.Prepare("INSERT INTO metadata " +
		"(key, value) VALUES ($1, $2)")
	if err != nil {
		return
	}
	defer stmt.Close()

	_, err = stmt.Exec(key, value)
	return
}

func getVersion(db *sql.DB) (version int, err error) {
	s, err := metaGetValue(db, VERSION_FIELD)
	if err != nil {
		return
	}

	version, err = strconv.Atoi(s)
	return
}

func addToLog(db *sql.DB, q *qemu.QemuSystem, ka config.Artifact,
	ki config.KernelInfo, res *phasesResult) (err error) {

	stmt, err := db.Prepare("INSERT INTO log (name, type, " +
		"distro_type, distro_release, kernel_release, " +
		"build_output, build_ok, " +
		"run_output, run_ok, " +
		"test_output, test_ok, " +
		"qemu_stdout, qemu_stderr, " +
		"kernel_panic, timeout_kill) " +
		"VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, " +
		"$10, $11, $12, $13, $14, $15);")
	if err != nil {
		return
	}

	defer stmt.Close()

	_, err = stmt.Exec(
		ka.Name, ka.Type,
		ki.DistroType, ki.DistroRelease, ki.KernelRelease,
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

func getAllLogs(db *sql.DB, num int) (les []logEntry, err error) {
	stmt, err := db.Prepare("SELECT id, time, name, type, " +
		"distro_type, distro_release, kernel_release, " +
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
		le := logEntry{}
		err = rows.Scan(&le.ID, &le.Timestamp,
			&le.Name, &le.Type,
			&le.DistroType, &le.DistroRelease, &le.KernelRelease,
			&le.Build.Ok, &le.Run.Ok, &le.Test.Ok,
			&le.KernelPanic, &le.KilledByTimeout,
		)
		if err != nil {
			return
		}

		les = append(les, le)
	}

	return
}

func getAllArtifactLogs(db *sql.DB, num int, ka config.Artifact) (
	les []logEntry, err error) {

	stmt, err := db.Prepare("SELECT id, time, name, type, " +
		"distro_type, distro_release, kernel_release, " +
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
		le := logEntry{}
		err = rows.Scan(&le.ID, &le.Timestamp,
			&le.Name, &le.Type,
			&le.DistroType, &le.DistroRelease, &le.KernelRelease,
			&le.Build.Ok, &le.Run.Ok, &le.Test.Ok,
			&le.KernelPanic, &le.KilledByTimeout,
		)
		if err != nil {
			return
		}

		les = append(les, le)
	}

	return
}

func getLogByID(db *sql.DB, id int) (le logEntry, err error) {
	stmt, err := db.Prepare("SELECT id, time, name, type, " +
		"distro_type, distro_release, kernel_release, " +
		"build_ok, run_ok, test_ok, " +
		"build_output, run_output, test_output, " +
		"qemu_stdout, qemu_stderr, " +
		"kernel_panic, timeout_kill " +
		"FROM log WHERE id=$1")
	if err != nil {
		return
	}
	defer stmt.Close()

	err = stmt.QueryRow(id).Scan(&le.ID, &le.Timestamp,
		&le.Name, &le.Type,
		&le.DistroType, &le.DistroRelease, &le.KernelRelease,
		&le.Build.Ok, &le.Run.Ok, &le.Test.Ok,
		&le.Build.Output, &le.Run.Output, &le.Test.Output,
		&le.Stdout, &le.Stderr,
		&le.KernelPanic, &le.KilledByTimeout,
	)
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

	exists, _ := metaChkValue(db, VERSION_FIELD)
	if !exists {
		err = createSchema(db)
		if err != nil {
			return
		}

		err = metaSetValue(db, VERSION_FIELD,
			strconv.Itoa(CURRENT_DATABASE_VERSION))
		return
	}

	version, err := getVersion(db)
	if err != nil {
		return
	}

	if version != CURRENT_DATABASE_VERSION {
		err = fmt.Errorf("Database is not supported (%d instead of %d)",
			version, CURRENT_DATABASE_VERSION)
		return
	}

	return
}
