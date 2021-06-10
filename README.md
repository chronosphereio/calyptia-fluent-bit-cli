# Fluent Bit CLI

### CI Status

| CI Workflow       | Status             |
|-------------------|--------------------|
| Unit Tests (main) | [![CI/Unit Tests](https://github.com/calyptia/fluent-bit-cli/actions/workflows/main-branch.yml/badge.svg?branch=main)](https://github.com/calyptia/fluent-bit-cli/actions/workflows/main-branch.yml) |


Fluent Bit Client UI Tool.

[![asciicast](https://asciinema.org/a/419611.svg)](https://asciinema.org/a/419611)

## Build instructions

Requires [Go](https://golang.org/) 1.16 to be installed.

```bash
make common-build
```

### Running the cli

Built binary
```bash
./fluent-bit-cli
```

Via snap

```bash
sudo snap install fluent-bit-cli --edge
```

### Running Tests

```bash
make test
```

## Keyboard shortcuts

Move with arrow keys <kbd>up</kbd> and <kbd>down</kbd> to select the chart you want to see.<br>
Hit <kbd>Ctrl+C</kbd> to exit.
