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

func TestDockerCommand(t *testing.T) {
	tmp, err := ioutil.TempDir("/tmp/", "out-of-tree_test_")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	start := time.Now()

	c := dockerCommand("ubuntu", tmp, "1s", "sleep 5s")
	_, err = c.CombinedOutput()
	if err == nil {
		t.Fatal("docker is not killed by timeout")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal(fmt.Sprintf("timeout failed (%v instead of %v)",
			time.Since(start), time.Second))
	}

	c = dockerCommand("ubuntu", tmp, "1m", "echo hello")
	rawOutput, err := c.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(rawOutput), "hello") {
		t.Fatal("wrong output (" + string(rawOutput) + ")")
	}
}
