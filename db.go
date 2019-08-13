// Copyright 2019 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"

	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/qemu"
)

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

		kernel_panic	BOOLEAN,
		timeout_kill	BOOLEAN
	)`)
	return
}

func addToLog(db *sql.DB, q *qemu.QemuSystem, ka config.Artifact,
	ki config.KernelInfo, res *phasesResult) (err error) {

	stmt, err := db.Prepare("INSERT INTO log (name, type, " +
		"distro_type, distro_release, kernel_release, " +
		"build_output, build_ok, " +
		"run_output, run_ok, " +
		"test_output, test_ok, " +
		"kernel_panic, timeout_kill) " +
		"VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, " +
		"$10, $11, $12, $13);")
	if err != nil {
		return
	}

	defer stmt.Close()

	_, err = stmt.Exec(
		ka.Name, ka.Type.String(),
		ki.DistroType.String(), ki.DistroRelease, ki.KernelRelease,
		res.Build.Output, res.Build.Ok,
		res.Run.Output, res.Run.Ok,
		res.Test.Output, res.Test.Ok,
		q.KernelPanic, q.KilledByTimeout,
	)
	if err != nil {
		return
	}

	return
}

func createSchema(db *sql.DB) (err error) {
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

	err = createSchema(db)
	if err != nil {
		return
	}

	return
}
