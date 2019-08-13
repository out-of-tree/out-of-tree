// Copyright 2019 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"database/sql"
	"fmt"
	"math"

	"code.dumpstack.io/tools/out-of-tree/config"
	"gopkg.in/logrusorgru/aurora.v1"
)

func logLogEntry(l logEntry) {
	distroInfo := fmt.Sprintf("%s-%s {%s}", l.DistroType,
		l.DistroRelease, l.KernelRelease)

	artifactInfo := fmt.Sprintf("{[%s] %s}", l.Type, l.Name)

	colored := ""
	if l.Type == config.KernelExploit {
		colored = aurora.Sprintf("[%s] %40s %40s: %s %s",
			l.Timestamp, artifactInfo, distroInfo,
			genOkFail("BUILD", l.Build.Ok),
			genOkFail("LPE", l.Test.Ok))
	} else {
		colored = aurora.Sprintf("[%s] %40s %40s: %s %s %s",
			l.Timestamp, artifactInfo, distroInfo,
			genOkFail("BUILD", l.Build.Ok),
			genOkFail("INSMOD", l.Run.Ok),
			genOkFail("TEST", l.Test.Ok))
	}

	additional := ""
	if l.KernelPanic {
		additional = "(panic)"
	} else if l.KilledByTimeout {
		additional = "(timeout)"
	}

	if additional != "" {
		fmt.Println(colored, additional)
	} else {
		fmt.Println(colored)
	}
}

func logHandler(db *sql.DB, path string, num int, rate bool) (err error) {
	var les []logEntry

	ka, kaErr := config.ReadArtifactConfig(path + "/.out-of-tree.toml")
	if kaErr == nil {
		les, err = getAllArtifactLogs(db, num, ka)
	} else {
		les, err = getAllLogs(db, num)
	}
	if err != nil {
		return
	}

	s := "\nS"
	if rate {
		if kaErr != nil {
			err = kaErr
			return
		}

		s = fmt.Sprintf("{[%s] %s} Overall s", ka.Type, ka.Name)

		les, err = getAllArtifactLogs(db, math.MaxInt64, ka)
		if err != nil {
			return
		}
	} else {
		for _, l := range les {
			logLogEntry(l)
		}
	}

	success := 0
	for _, l := range les {
		if l.Test.Ok {
			success += 1
		}
	}

	overall := float64(success) / float64(len(les))
	fmt.Printf("%success rate: %.04f (~%.0f%%)\n",
		s, overall, overall*100)

	return
}
