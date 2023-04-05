module code.dumpstack.io/tools/out-of-tree

go 1.14

replace code.dumpstack.io/tools/out-of-tree/qemu => ./qemu

replace code.dumpstack.io/tools/out-of-tree/config => ./config

require (
	github.com/alecthomas/kong v0.7.1
	github.com/go-git/go-git/v5 v5.6.1
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/mattn/go-sqlite3 v1.14.16
	github.com/mitchellh/go-homedir v1.1.0
	github.com/naoina/go-stringutil v0.1.0 // indirect
	github.com/naoina/toml v0.1.1
	github.com/olekukonko/tablewriter v0.0.5
	github.com/otiai10/copy v1.9.0
	github.com/remeh/sizedwaitgroup v1.0.0
	github.com/rs/zerolog v1.29.0
	github.com/zcalusic/sysinfo v0.9.5
	golang.org/x/crypto v0.7.0
	gopkg.in/logrusorgru/aurora.v2 v2.0.3
)
