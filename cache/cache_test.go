package cache

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"code.dumpstack.io/tools/out-of-tree/fs"
)

func TestDownloadQemuImage(t *testing.T) {

	tmp, err := ioutil.TempDir("", "out-of-tree_")
	if err != nil {
		return
	}
	defer os.RemoveAll(tmp)

	file := "out_of_tree_ubuntu_12__04.img"

	err = DownloadQemuImage(tmp, file)
	if err != nil {
		t.Fatal(err)
	}

	if !fs.PathExists(filepath.Join(tmp, file)) {
		t.Fatalf("%s does not exist", file)
	}
}
