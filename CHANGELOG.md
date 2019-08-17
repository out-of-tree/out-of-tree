# Changelog

[ISO 8601](https://xkcd.com/1179/).

[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- New parameter `--max=X` is added for `autogen` (generate kernels
  base on `.out-of-tree.toml` definitions) and `pew` (automated runs)
  and allows to specify a maximum number of runs per each supported
  kernel in module/exploit definition.

- New command `genall` -- generate all kernels for specified
  distro/version.

- All logs stores in sqlite3 database. Implemented specific commands
  for making simple queries and export data to markdown and json.

- Implemented success rate calculation for previous runs.

- Save of build results supported by parameter `--dist` for `pew`.

- Support for generating kernels info from host system.

- Support for build on host.

- Support for custom kernels.

- Now debugging environment is automatically looking for debug kernel
  on the host system.

- Added ability to enable/disable kaslr/smep/smap for debugging by
  command line flags.

- New parameter `--threads=N` is added for `pew` and allows to specify
  maximum number of threads that will be used for parallel
  build/run/test.

- Tagging for runs. Tags write to log and can be used for statistics.

### Changed

- Now if there's no base image found — out-of-tree will try to use an
  image from closest previous version, e.g. image from Ubuntu 18.04
  for Ubuntu 18.10.

- Kernel modules tests will not be failed if there are no tests
  exists.

- Now *out-of-tree* will return negative error code if at least one of
  the stage was failed.

- Project is switch to use Go modules.

### Removed

- *Kernel factory* is removed completely in favor of incremental
  Dockerfiles.

### Fixed

- Command `timeout` is not required anymore.

- Errors is more meaningful.

- Temporary files is moved to `~/.out-of-tree/tmp/` to avoid docker mounting issues on some systems.

## [0.2.0] - 2019-12-01

The main purpose of the release is to simplify installation.

### Changes
- All configuration moved to `~/.out-of-tree`.
- Now prebuilt images can be downloaded with bootstrap.
- Ability to generate kernels specific to .out-of-tree.toml in current
  directory. So now there's no need to wait for several hours for
  start work on specific kernel with module/exploit.
- Now there's no need to keep source tree and _out-of-tree_ can be
  distributed in binary form.
- New command: **debug**. Creates interactive environment for kernel
  module/exploit development. Still work-in-progress.

## [0.1.0] - 2019-11-20

Initial release that was never tagged.

Refer to state after first public release on ZeroNights 2018
([video](https://youtu.be/2tL7bbCdIio),
[slides](https://2018.zeronights.ru/wp-content/uploads/materials/07-Ways-to-automate-testing-Linux-kernel-exploits.pdf)).
