package dotfiles

import (
	"os"
	"os/user"
	"path/filepath"

	"github.com/rs/zerolog/log"
)

// Directory for config files
var Directory string

func directory() string {
	if Directory != "" {
		return Directory
	}

	usr, err := user.Current()
	if err != nil {
		log.Fatal().Err(err).Msg("get current user")
	}

	Directory = filepath.Join(usr.HomeDir, ".out-of-tree")

	return Directory
}

// Dir that exist relative to config directory
func Dir(s ...string) (dir string) {
	dir = filepath.Join(append([]string{directory()}, s...)...)
	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		log.Fatal().Err(err).Msg("mkdir")
	}
	return
}

// File in existing dir relative to config directory
func File(s ...string) (file string) {
	file = filepath.Join(append([]string{directory()}, s...)...)
	err := os.MkdirAll(filepath.Dir(file), os.ModePerm)
	if err != nil {
		log.Fatal().Err(err).Msg("mkdir")
	}
	return
}
