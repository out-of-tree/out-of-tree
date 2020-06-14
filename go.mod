module code.dumpstack.io/tools/out-of-tree

go 1.14

replace code.dumpstack.io/tools/out-of-tree/qemu => ./qemu

replace code.dumpstack.io/tools/out-of-tree/config => ./config

require (
	github.com/alecthomas/template v0.0.0-20160405071501-a0175ee3bccc // indirect
	github.com/alecthomas/units v0.0.0-20151022065526-2efee857e7cf // indirect
	github.com/go-git/go-git/v5 v5.1.0
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/mattn/go-runewidth v0.0.4 // indirect
	github.com/mattn/go-sqlite3 v1.11.0
	github.com/naoina/go-stringutil v0.1.0 // indirect
	github.com/naoina/toml v0.1.1
	github.com/olekukonko/tablewriter v0.0.1
	github.com/otiai10/copy v1.0.1
	github.com/otiai10/curr v1.0.0 // indirect
	github.com/remeh/sizedwaitgroup v0.0.0-20180822144253-5e7302b12cce
	github.com/stretchr/testify v1.5.1 // indirect
	github.com/zcalusic/sysinfo v0.0.0-20190429151633-fbadb57345c2
	golang.org/x/crypto v0.0.0-20200302210943-78000ba7a073
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/logrusorgru/aurora.v2 v2.0.0-20190417123914-21d75270181e
)
