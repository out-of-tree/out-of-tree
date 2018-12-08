// Copyright 2018 Mikhail Klementev. All rights reserved.
// Use of this source code is governed by a AGPLv3 license
// (or later) that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDockerRun(t *testing.T) {
	tmp, err := ioutil.TempDir("/tmp/", "out-of-tree_test_")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	start := time.Now()

	timeout := time.Second

	_, err = dockerRun(timeout, "ubuntu", tmp, "sleep 5s")
	if err == nil {
		t.Fatal("docker is not killed by timeout")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal(fmt.Sprintf("timeout failed (%v instead of %v)",
			time.Since(start), time.Second))
	}

	output, err := dockerRun(time.Minute, "ubuntu", tmp, "echo hello")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(output, "hello") {
		t.Fatal("wrong output (" + output + ")")
	}
}
