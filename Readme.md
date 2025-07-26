# pT Tests

Basic example of how to do an alternative firmware for the picoTracker using TinyGo.

## Setup

Only tested on Ubuntu 24.04
First:

* install Go
* install tinygo

For debugging

* install gdb-multiarch
* install openocd

### Setup to use VSCode

* Install Go extension
* Install TinyGo extension
* Install Cortex-Debug extension (to use debugger)

## How to build

```
tinygo build -o out.elf -target pico -size short -opt 0 -serial uart ./test_firmware/hw.go
```

to flash, put pT into bootsel and then run:
```
tinygo flash
```

## VSCode

See docs/example.code-workspace for an example of how to run in VSCode under openocd+gdb via a picoprobe instead of needing to constantly flash a uf2 manually via mounting as usbdrive.

Use normal `F5` to build and debug in VSCode.

## Required tinygo dependencies

For i2s PIO see go.mod