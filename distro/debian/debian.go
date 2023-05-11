package debian

type CodeName int

const (
	Wheezy CodeName = iota
	Jessie
	Stretch
	Buster
	Bullseye
	Bookworm
)

var CodeNameStrings = [...]string{
	"Wheezy",
	"Jessie",
	"Stretch",
	"Buster",
	"Bullseye",
	"Bookworm",
}

func (cn CodeName) String() string {
	return CodeNameStrings[cn]
}
