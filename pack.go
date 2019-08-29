// Copyright 2019 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"runtime"
	"time"

	"code.dumpstack.io/tools/out-of-tree/config"
)

func packHandler(db *sql.DB, path, registry string, kcfg config.KernelConfig,
	autogen, download bool, exploitRuns, kernelRuns int64) (err error) {

	dockerTimeout := time.Minute
	qemuTimeout := time.Minute
	threads := runtime.NumCPU()

	tag := fmt.Sprintf("pack_run_%d", time.Now().Unix())
	log.Println("Tag:", tag)

	files, err := ioutil.ReadDir(path)
	if err != nil {
		return
	}

	for _, f := range files {
		workPath := path + "/" + f.Name()

		if !exists(workPath + "/.out-of-tree.toml") {
			continue
		}

		if autogen {
			var perRegex int64 = 1
			err = kernelAutogenHandler(workPath, registry,
				perRegex, false, download)
			if err != nil {
				return
			}
		}

		log.Println(f.Name())

		pewHandler(kcfg, workPath, "", "", "", false,
			dockerTimeout, qemuTimeout,
			kernelRuns, exploitRuns, pathDevNull, tag, threads, db)
	}

	return
}
