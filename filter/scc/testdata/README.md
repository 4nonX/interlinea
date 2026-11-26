# SCC test files

Files made by other tools, so the reader is tested against more than the
files its own tests write:

- `pycaption/`: the Scenarist caption samples of pycaption's tests
  (https://github.com/pbs/pycaption, commit b8d3e11), taken out of its test
  fixtures as they are. Many are written to trip decoders up: addresses
  that restate a row, rows past the last column, roll-up windows, paint-on,
  mid-row codes. Apache License 2.0; see `pycaption/LICENSE`. The rows
  pycaption's tests expect of them are checked in `files_test.go`.
- `fuzz/`: inputs the fuzz test found problems with, kept so they stay
  fixed.
