# Dependency license policy

The repository license is MIT. The automated dependency check permits these
SPDX license identifiers:

- Apache-2.0
- BSD-2-Clause
- BSD-3-Clause
- ISC
- MIT
- MIT-0
- MPL-2.0
- Unicode-3.0

This list is an engineering policy. It is not legal advice or a compatibility
assurance.

## Complete packaged licenses

`github.com/mattn/go-localereader v0.0.1` declared MIT but omitted the full
license text. This caused the dependency license check to fail on native
Windows, where the dependency enters the build.

The repository pins `v0.0.2-0.20220822084749-2491eb6c1c75`, which includes the
[upstream license-only commit](https://github.com/mattn/go-localereader/commit/2491eb6c1c75).
That commit adds the complete MIT license without changing source code.
The dependency uses the normal license check, without an exception.

## Tool exceptions

`modernc.org/mathutil v1.7.1` is ignored by `go-licenses v1.6.0` because that
tool does not classify the packaged license at its default confidence. The
module contains a BSD-3-Clause text in `LICENSE`.

The `make licenses` command fails unless:

- the selected module version is exactly `v1.7.1`; and
- the SHA-256 of its packaged `LICENSE` is exactly
  `bfa9bf72a72ca009fd62a8f84fca3dca67e51d93af96352723646599898b6cf5`.

Owner: repository maintainer.

Review deadline: 2026-11-29 or the next `modernc.org/mathutil` or license-tool
upgrade, whichever comes first. Remove the exception when the tool classifies
the dependency at its default confidence.

`github.com/segmentio/asm v1.2.1` uses MIT-0. `go-licenses v1.6.0` does not
classify its packaged MIT-0 text. The `make licenses` command fails unless:

- the selected module version is exactly `v1.2.1`; and
- the SHA-256 of its packaged `LICENSE` is exactly
  `cca993712df289a5958bdef69031a5dac0f951ac15afeb313f9eeea55ed59443`.

Owner: repository maintainer.

Review deadline: 2026-11-29 or the next `github.com/segmentio/asm` or
license-tool upgrade, whichever comes first. Remove the exception when the tool
classifies MIT-0 at its default confidence.
