package cmd

import (
	"net/url"

	"code.dumpstack.io/tools/out-of-tree/config"
)

type Globals struct {
	Config config.OutOfTree `help:"path to out-of-tree configuration" default:"~/.out-of-tree/out-of-tree.toml"`

	WorkDir string `help:"path to work directory" default:"./" type:"path" existingdir:""`

	CacheURL url.URL

	Remote     bool   `help:"run at remote server"`
	RemoteAddr string `default:"localhost:63527"`
}
