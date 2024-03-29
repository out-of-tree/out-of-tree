package cache

import (
	"os"
	"path/filepath"
	"testing"

	"code.dumpstack.io/tools/out-of-tree/fs"
)

func TestDownloadRootFS(t *testing.T) {
	tmp, err := os.MkdirTemp("", "out-of-tree_")
	if err != nil {
		return
	}
	defer os.RemoveAll(tmp)

	file := "out_of_tree_ubuntu_12__04.img"

	err = DownloadRootFS(tmp, file)
	if err != nil {
		t.Fatal(err)
	}

	if !fs.PathExists(filepath.Join(tmp, file)) {
		t.Fatalf("%s does not exist", file)
	}
}

func TestDownloadDebianCache(t *testing.T) {
	tmp, err := fs.TempDir()
	if err != nil {
		return
	}
	defer os.RemoveAll(tmp)

	file := "debian.cache"

	cachePath := filepath.Join(tmp, file)

	err = DownloadDebianCache(cachePath)
	if err != nil {
		t.Fatal(err)
	}

	if !fs.PathExists(filepath.Join(tmp, file)) {
		t.Fatalf("%s does not exist", file)
	}
}
