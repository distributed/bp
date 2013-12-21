// Copyright 2012 Michael Meier. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package bp enables access to a bus pirate. Currently only I2C mode
// is implemented.
package bp

import (
	"bytes"
	"errors"
	"fmt"
	"io"
)

type timeoutError interface {
	error
	Timeout() bool
}

func isTimeout(err error) bool {
	if terr, ok := err.(timeoutError); ok {
		return terr.Timeout()
	}
	return false
}

// Conn represents a serial connection to a bus pirate. The connection
// needs to be open and configured with the correct baud rate prior
// to use.
//
// The signature
// is satisfied by sers.SerialPort. Find it at github.com/distributed/sers.
type Conn interface {
	io.ReadWriteCloser
	SetReadParams(int, float64) error
}

// BusPirate offers the functionality of a bus pirate. It works with
// the device in binary mode. Before using a BusPirate object, the user
// has to put the bus pirate into a known state via a call to
// BusPirate.Open().
type BusPirate struct {
	c           Conn
	mode        int
	modeversion int
}

// NewBusPirate generates a new BusPirate objected that uses c as its
// communication channel. Note that you need to correctly configure the
// connection with respect to baud rate, parity and flow control. An 8 bit
// connection is assumed. The BusPirate object pointed to by the return value
// is not ready to use, you need to call the Open() method to put the device
// into a known state.
func NewBusPirate(c Conn) *BusPirate {
	return &BusPirate{c: c}
}

// Open puts the bus pirate into binary bit bang mode. The user needs
// to call this method as the bus pirate cannot be assumed to be in
// any specific mode when the connection to it is opened.
func (bp *BusPirate) Open() error {
	err := bp.c.SetReadParams(0, 100e-3)
	if err != nil {
		return err
	}

	var bbuf [1]byte
	for i := 0; i < 20; i++ {
		fmt.Printf("try % 2d: sending 0x00...\n", i)
		bbuf[0] = 0x00
		_, err := bp.c.Write(bbuf[0:])
		if err != nil {
			return err
		}

		rbuf := make([]byte, 2048)
		//n, err := bp.c.Read(rbuf)
		_, err = io.ReadFull(bp.c, rbuf[0:5])
		if err != nil {
			if isTimeout(err) {
				fmt.Printf("\ttimeout!\n")
				continue
			}
			return err
		}

		fmt.Printf("buf %q\n", rbuf[0:5])
		if !bytes.HasPrefix(rbuf, []byte("BBIO")) {
			return fmt.Errorf("response does not start with 'BBIO'")
		}

		if rbuf[4] != '1' {
			return fmt.Errorf("protocol mismatch: only support protocol '1', bus pirate uses protocol %q", rbuf[4])
		}

		// parsed BBIO1

		// drain buffer

		err = bp.c.SetReadParams(0, 0.3)
		if err != nil {
			return err
		}

		n, err := io.ReadFull(bp.c, rbuf)
		if !isTimeout(err) {
			return err
		}
		fmt.Printf("drained buffer, %d excess bytes discarded\n", n)

		bp.mode = MODE_BITBANG
		bp.modeversion = 1
		return nil
	}

	return fmt.Errorf("bp: no suitable response after maximum number of trials\n")
}

// Close leaves binary mode. If the bus pirate is currently not in
// binary bit bang mode, it first enters binary bit bang mode. If the
// user does not call Close, the device might be unresponsive in text
// mode.
func (bp *BusPirate) Close() error {
	if bp.mode == MODE_UNKNOWN {
		return fmt.Errorf("cannot leave unknown mode")
	}

	if bp.mode != MODE_BITBANG {
		fmt.Printf("need to go to bitbang mode before closing\n")
		err := bp.EnterBitbangMode()
		if err != nil {
			return fmt.Errorf("could not enter bitbang mode to close connection: %v", err)
		}
	}

	r, err := bp.exchangeByte(0x0f)
	if err != nil {
		return err
	}

	if r != 0x01 {
		return fmt.Errorf("*BusPirate.Close(): expected response 0x01, got %#02x\n", r)
	}

	fmt.Printf("bp closed\n")

	return nil
}

func (bp *BusPirate) writeByte(b byte) error {
	sl := []byte{b}
	if _, err := bp.c.Write(sl); err != nil {
		errors.New("write byte to bus pirate: " + err.Error())
	}
	return nil
}

func (bp *BusPirate) readByte() (byte, error) {
	sl := make([]byte, 1)
	n, err := bp.c.Read(sl)
	if n != 1 || err != nil {
		return 0, errors.New("read from bus pirate: " + err.Error())
	}
	return sl[0], nil
}

func (bp *BusPirate) exchangeByte(in byte) (byte, error) {
	err := bp.writeByte(in)
	if err != nil {
		return 0x00, err
	}
	return bp.readByte()
}

func (bp *BusPirate) exchangeByteAndExpect(in byte, exp byte) error {
	rb, err := bp.exchangeByte(in)
	if err != nil {
		return err
	}

	if rb != exp {
		return fmt.Errorf("unexpected response from bus pirate, got %#02x, want %#02x", rb, exp)
	}

	return nil
}

func (bp *BusPirate) EnterBitbangMode() error {
	if bp.mode == MODE_UNKNOWN {
		return fmt.Errorf("cannot enter bitbang mode from unknown mode")
	}

	err := bp.writeByte(0x00)
	if err != nil {
		bp.clearMode()
		return err
	}

	var rb [5]byte
	_, err = io.ReadFull(bp.c, rb[0:])
	if err != nil {
		bp.clearMode()
		return fmt.Errorf("error reading response: %v", err)
	}

	if !bytes.HasPrefix(rb[0:], []byte("BBIO")) {
		bp.clearMode()
		return fmt.Errorf("expected version string \"BBIOx\", got %q", rb)
	}

	if rb[4] != '1' {
		bp.clearMode()
		return fmt.Errorf("only BBIO version 1 is supported, bus pirate uses version %q", rb[4])
	}

	return nil
}
