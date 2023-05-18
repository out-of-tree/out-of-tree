package distro

import (
	"fmt"
	"strings"
)

// ID of the distro
type ID int

const (
	// Ubuntu https://ubuntu.com/
	Ubuntu ID = iota
	// CentOS https://www.centos.org/
	CentOS
	// Debian https://www.debian.org/
	Debian
	// OracleLinux https://www.oracle.com/linux/
	OracleLinux
)

var IDs = []ID{
	Ubuntu, CentOS, Debian, OracleLinux,
}

var nameStrings = [...]string{
	"Ubuntu",
	"CentOS",
	"Debian",
	"OracleLinux",
}

func NewID(name string) (id ID, err error) {
	err = id.UnmarshalTOML([]byte(name))
	return
}

func (id ID) String() string {
	return nameStrings[id]
}

// UnmarshalTOML is for support github.com/naoina/toml
func (id *ID) UnmarshalTOML(data []byte) (err error) {
	name := strings.Trim(string(data), `"`)
	if strings.EqualFold(name, "Ubuntu") {
		*id = Ubuntu
	} else if strings.EqualFold(name, "CentOS") {
		*id = CentOS
	} else if strings.EqualFold(name, "Debian") {
		*id = Debian
	} else if strings.EqualFold(name, "OracleLinux") {
		*id = OracleLinux
	} else {
		err = fmt.Errorf("distro %s is unsupported", name)
	}
	return
}

// MarshalTOML is for support github.com/naoina/toml
func (id ID) MarshalTOML() (data []byte, err error) {
	data = []byte(`"` + id.String() + `"`)
	return
}

type Distro struct {
	ID      ID
	Release string
}
