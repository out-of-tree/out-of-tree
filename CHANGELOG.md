# Changelog

[ISO 8601](https://xkcd.com/1179/).

[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.0.0]

### Breaking

- Layers with kernels in containers have been abandoned in favor of
  installation to mounted volumes.

- Command line interface has been changed to alecthomas/kong.

### Added

- Command `kernel install` to install specific kernel.

- Command `containers` to manage containers.

- Command `image edit` to edit qemu image.

- Flag `--force` to force reinstallation of the kernel.

- Flag `--artifact-config` to specify the path to .out-of-tree.toml.

- Flag `--no-headers` flag to install kernel and initrd only.

- Flag `--shuffle` to randomize the order of kernels for
  installation/testing.

- Support make targets in artifact config.

- Support patches in artifact config.

- Support for copying standard modules to qemu.

- Script artifact type for various automation and information gathering.

- Add TestFiles to artifact config, transfers additional test files to VM.

- Improved logging, with logfile at ~/.out-of-tree/logs/out-of-tree.log

- Kernel installation will retry (10 times by default) in case of
  network problems.

- Stdout trace (with --log-level=trace, and always to logfile) for
  qemu and container execution.

- Compatibility with Podman.

- Support for Ubuntu 22.04.

## [1.4.0]

### Added

- Parameter `--docker-timeout` may also be set in the artifact
  configuration file.

- Preload modules before inserting module or run exploit. Modules can
  be specified by git repository path in the `repo` parameter of
  section `[[preload]]`. Also, there is a `path` parameter for local
  projects. Note that `repo` is using a cache that uses last commit
  hash to check is project needs to be rebuilt, so it's not suitable
  for local development (except if you will commit each time before
  run out-of-tree).

- Flag `--disable-preload` to ignore `[[preload]]` section of
  configuration file.

- Now `out-of-tree log dump` will show the last log if no ID
  specified.

## [1.3.0] 2020-05-30

### Added

- Support for Ubuntu 20.04 and CentOS 8.

## [1.2.1] 2019-12-25

### Fixed

- macOS support.

## [1.2.0] 2019-11-15

### Added

- Flag for Verbose output. Right now only qemu status messages is
  implemented.

### Fixed

- Kpti settings was not affected for regular runs.

## [1.1.2] 2019-09-05

### Added

- Added policykit-1 to rootfs for Ubuntu.

### Fixed

- Avoided slow mirrors with use of mirror://mirrors.ubuntu.com for
  Ubuntu 16.04 and newer.

## [1.1.1] 2019-08-31

### Fixed

- macOS support.

## [1.1.0] 2019-08-30

### Added

- Global configuration file (~/.out-of-tree/out-of-tree.toml) allow to
  set up default values for settings.

- rootfs generator for Ubuntu 14.04.

- Parameter for setting up docker registry server.

- Support for (distro-specific) custom docker commands that will be
  executed before the base template.

- Parameter for setting up a reliability threshold for exit code.

- Parameter for setting up global timeout, after which no new tasks
  will be started.

### Fixed

- Spelling in output.

- Now kernel generation will not fail if there are no directory
  /lib/modules inside the container.

## [1.0.0] 2019-08-20

### Added

- New parameter `--max=X` is added for `autogen` (generate kernels
  base on `.out-of-tree.toml` definitions) and `pew` (automated
  runs) and allows to specify a maximum number of runs per each
  supported kernel in module/exploit definition.

- New command `genall` -- generate all kernels for specified
  distro/version.

- All logs stores in sqlite3 database. Implemented specific commands
  for making simple queries and export data to markdown and json.

- Implemented success rate calculation for previous runs.

- Save of build results supported by parameter `--dist` for `pew`.

- Support for generating kernels info from host system.

- Support for build on host.

- Support for custom kernels.

- Now debugging environment is automatically looking for debug
  kernel on the host system.

- Added ability to enable/disable kaslr/smep/smap/kpti for debugging
  by command line flags.

- New parameter `--threads=N` is added for `pew` and allows to
  specify maximum number of threads that will be used for parallel
  build/run/test.

- Tagging for runs. Tags write to log and can be used for
  statistics.

- Added non-regex way to set kernel version in .out-of-tree.toml (see
  examples).

- New command `pack` that perform tests in subdirectories.

- Added ability to disable kaslr/smep/smap/kpti for in artifact
  definition.

- Added ability to change amount of memory/CPUs and set qemu timeout
  in artifact definition (`.out-of-tree.toml`).

- Now images downloading while `kernel autogen`, bootstrap is not
  required anymore.

- Support CentOS kernels.

### Changed

- Now if there's no base image found — out-of-tree will try to use
  an image from closest previous version, e.g. image from Ubuntu
  18.04 for Ubuntu 18.10.

- Kernel modules tests will not be failed if there are no tests
  exists.

- Now *out-of-tree* will return negative error code if at least one
  of the stage was failed.

- Project is switch to use Go modules.

- Now test.sh is used by default if copying is not implemented in
  Makefile.

- dmesg is not cleaned before the start of module/exploit anymore.

- qemu/kvm will use all host cpu features.

### Removed

- *Kernel factory* is removed completely in favor of incremental
  Dockerfiles.

- `bootstrap` is not doing anything anymore. It'll be removed in next
  release.

### Fixed

- Command `timeout` is not required anymore.

- Errors is more meaningful.

- Temporary files is moved to `~/.out-of-tree/tmp/` to avoid docker
  mounting issues on some systems.

## [0.2.0] - 2018-12-01

The main purpose of the release is to simplify installation.

### Changes

- All configuration moved to `~/.out-of-tree`.

- Now prebuilt images can be downloaded with bootstrap.

- Ability to generate kernels specific to .out-of-tree.toml in
  current directory. So now there's no need to wait for several
  hours for start work on specific kernel with module/exploit.

- Now there's no need to keep source tree and _out-of-tree_ can be
  distributed in binary form.

- New command: **debug**. Creates interactive environment for kernel
  module/exploit development. Still work-in-progress.

- No warning anymore if test.sh is not exists.

## [0.1.0] - 2018-11-20

Initial release that was never tagged.

Refer to state after first public release on ZeroNights 2018
([video](https://youtu.be/2tL7bbCdIio),
[slides](https://2018.zeronights.ru/wp-content/uploads/materials/07-Ways-to-automate-testing-Linux-kernel-exploits.pdf)).
