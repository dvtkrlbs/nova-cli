# nova-cli

Nova-cli is a command line tool that is used by GTU-nova aviation team for developing autonomous
behaviors to Inav based board using Multiwii Serial Protocol. The protocol library that is used is
[msp-lib](https://github.com/gtu-nova/msp-lib).

## Installation from source

To do so, install [Go](http://golang.org) in your system, then type the
following command:

```sh
  $ go get -v github.com/gtu-nova/nova-cli
```

msp-tool will be installed to ${GOPATH}/bin.

## Using msp-tool

To start msp-tool, the only required argument is `-p`, which indicates the
serial port it should open to connect to the flight controller. Once connected,
it will print some information about the firmware and the board. For example:

```
$ nova-cli -p /dev/tty.SLAB_USBtoUART 

Connected to /dev/tty.usbmodem14211 @ 115200bps. Press 'h' for help.
MSP API version 2.1 (protocol 0)
INAV 1.9.0 (board OBSD, target OMNIBUSF4PRO)
Build 2bcdc237 (built on Feb 16 2018 @ 23:16:49)
```