package fs

import (
	"os"
	"path/filepath"

	"code.dumpstack.io/tools/out-of-tree/config"
)

// CaseInsensitive check
func CaseInsensitive(dir string) (yes bool, err error) {
	pathLowercase := filepath.Join(dir, "file")
	fLowercase, err := os.Create(pathLowercase)
	if err != nil {
		return
	}
	defer fLowercase.Close()
	defer os.Remove(pathLowercase)

	pathUppercase := filepath.Join(dir, "FILE")
	fUppercase, err := os.Create(pathUppercase)
	if err != nil {
		return
	}
	defer fUppercase.Close()
	defer os.Remove(pathUppercase)

	statLowercase, err := fLowercase.Stat()
	if err != nil {
		return
	}

	statUppercase, err := fUppercase.Stat()
	if err != nil {
		return
	}

	yes = os.SameFile(statLowercase, statUppercase)
	return
}

// PathExists check
func PathExists(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return true
}

// TempDir that exist relative to config directory
func TempDir() (string, error) {
	return os.MkdirTemp(config.Dir("tmp"), "")
}
