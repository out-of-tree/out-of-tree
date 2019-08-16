# Changelog

[ISO 8601](https://xkcd.com/1179/).

[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

TODO

## [0.2.0] - 2019-12-01

The main purpose of the release is to simplify installation.

## Changes
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

