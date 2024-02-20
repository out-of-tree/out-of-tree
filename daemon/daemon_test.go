package daemon

import (
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func init() {
	log.Logger = zerolog.New(zerolog.ConsoleWriter{
		Out:     os.Stdout,
		NoColor: true,
	})
}
