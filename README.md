# Fluent Bit CLI

### CI Status

| CI Workflow       | Status             |
|-------------------|--------------------|
| Unit Tests (main) | [![CI/Unit Tests](https://github.com/calyptia/fluent-bit-cli/actions/workflows/main-branch.yml/badge.svg?branch=main)](https://github.com/calyptia/fluent-bit-cli/actions/workflows/main-branch.yml) |


Fluent Bit Client UI Tool.

![Screenshot](https://raw.githubusercontent.com/calyptia/fluent-bit-cli/main/assets/screenshot_0_20210531.png)

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
