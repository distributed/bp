// Copyright 2012 Michael Meier. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package bp

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/distributed/i2cm"
	"io"
)

// BusPirateI2C represents a bus pirate in I2C mode. It offers an
// interface consistent with i2cm.I2CMaster. Obtain a BusPirateI2C by switching
// the bus pirate into I2C mode with *BusPirate.EnterI2CMode().
// When the user makes
// the bus pirate switch into a different mode, the BusPirateI2C
// object becomes invalid and must no be used any longer.
type BusPirateI2C struct {
	bp *BusPirate
}

// NonStrictI2C offers the same functionality as BusPirateI2C, but also
// adds a fast if not completely faithful Transact8x8 implementation.
// The Transact8x8 implementation is not faithful in that one transaction
// results in *two* transactions on the bus. Before using NonStrictI2C make
// sure that no other master is interfering with you and that your device's
// behavior doesn't change when a write-then-read transaction is split into
// a write and a read. Expect substantial speed gains from using this
// implementation.
//
// Obtain a NonStrictI2C by switching the bus pirate into non-strict I2C mode
// with *BusPirate.EnterNonStrictI2CMode().
type NonStrictI2C struct {
	BusPirateI2C
}

const (
	bpcmd_ENTER_I2C_MODE = 0x02
)

const (
	bpans_OK = 0x01
)

type i2cerror struct {
	Op  string
	Err error
}

func (e *i2cerror) Error() string {
	return e.Op + ": " + e.Err.Error()
}

// EnterI2CMode makes the bus pirate enter I2C mode and returns a
// BusPirateI2C object offering the I2C functionality of the device. 
// The I2CMode can only be entered from bitbang mode.
// This might change.
func (bp *BusPirate) EnterI2CMode() (BusPirateI2C, error) {
	var bpi2c BusPirateI2C

	if bp.mode != MODE_BITBANG {
		return bpi2c, ModeError("I2C mode can only be entered from raw bitbang mode")
	}

	err := bp.writeByte(bpcmd_ENTER_I2C_MODE)
	if err != nil {
		bp.clearMode()
		return bpi2c, err
	}

	var rb [4]byte
	_, err = io.ReadFull(bp.c, rb[0:])
	if err != nil {
		bp.clearMode()
		return bpi2c, fmt.Errorf("error reading response: %v", err)
	}

	if !bytes.HasPrefix(rb[0:], []byte("I2C")) {
		bp.clearMode()
		return bpi2c, fmt.Errorf("expected version string \"I2Cx\", got %q", rb)
	}

	if rb[3] != '1' {
		bp.clearMode()
		return bpi2c, fmt.Errorf("only I2C version 1 is supported, bus pirate uses version %q", rb[3])
	}

	bp.mode = MODE_I2C
	bp.modeversion = 1

	bpi2c.bp = bp

	return bpi2c, nil
}

var notI2CMode = ModeError("not in I2C mode")

const (
	bpcmd_I2C_START      = 0x02
	bpcmd_I2C_STOP       = 0x03
	bpcmd_I2C_READ       = 0x04
	bpcmd_I2C_ACK        = 0x06
	bpcmd_I2C_NACK       = 0x07
	bpcmd_I2C_WnR        = 0x08 // write then read
	bpcmd_I2C_BULK_WRITE = 0x10
)

const (
	i2c_RnW_MAXREAD  = 4096
	i2c_RnW_MAXWRITE = 4096
)

func (inf BusPirateI2C) Start() error {
	bp := inf.bp
	if bp.mode != MODE_I2C {
		return notI2CMode
	}

	if err := bp.exchangeByteAndExpect(bpcmd_I2C_START, bpans_OK); err != nil {
		return &i2cerror{"i2c.Start", err}
	}
	return nil
}

func (inf BusPirateI2C) Stop() error {
	bp := inf.bp
	if bp.mode != MODE_I2C {
		return notI2CMode
	}

	if err := bp.exchangeByteAndExpect(bpcmd_I2C_STOP, 0x01); err != nil {
		return &i2cerror{"i2c.Stop", err}
	}
	return nil
}

func (inf BusPirateI2C) ReadByte(ack bool) (byte, error) {
	bp := inf.bp
	if bp.mode != MODE_I2C {
		return 0x00, notI2CMode
	}

	b, err := bp.exchangeByte(bpcmd_I2C_READ)
	if err != nil {
		return 0, &i2cerror{"i2c.ReadByte", err}
	}

	if ack {
		err = bp.exchangeByteAndExpect(bpcmd_I2C_ACK, bpans_OK)
	} else {
		err = bp.exchangeByteAndExpect(bpcmd_I2C_NACK, bpans_OK)
	}

	if err != nil {
		err = &i2cerror{"i2c.ReadByte", err}
	}

	return b, err
}

func (inf BusPirateI2C) WriteByte(b byte) error {
	bp := inf.bp
	if bp.mode != MODE_I2C {
		return notI2CMode
	}

	// TODO: factor into bulk write

	//  bulk write cmd | count-1
	cmd := byte(bpcmd_I2C_BULK_WRITE | 0x00)
	if err := bp.exchangeByteAndExpect(cmd, bpans_OK); err != nil {
		return &i2cerror{"i2c.WriteByte", err}
	}

	ackb, err := bp.exchangeByte(b)
	if err != nil {
		return &i2cerror{"i2c.WriteByte", err}
	}

	if ackb != 0 {
		return i2cm.NACKReceived
	}

	return nil
}

func (bp *BusPirate) EnterNonStrictI2CMode() (NonStrictI2C, error) {
	// TODO: increase bp timeout? times out on ~4k transaction
	m, err := bp.EnterI2CMode()
	if err != nil {
		return NonStrictI2C{}, err
	}

	return NonStrictI2C{m}, nil
}

func (nsi NonStrictI2C) writeThenRead(w, r []byte) error {
	bp := nsi.bp

	// we have to write:
	// command: 1 byte
	// write count: 2 bytes, big endian
	// read count: 2 bytes, big endian
	// slice w: len(w) bytes
	if len(w) > i2c_RnW_MAXWRITE {
		return fmt.Errorf("bp.writeThenRead: cannot write more than %d bytes", i2c_RnW_MAXWRITE)
	}

	if len(r) > i2c_RnW_MAXREAD {
		return fmt.Errorf("bp.writeThenRead: cannot write more than %d bytes", i2c_RnW_MAXREAD)
	}

	header := make([]byte, 5)
	header[0] = bpcmd_I2C_WnR
	header[1] = uint8(len(w) >> 8)
	header[2] = uint8(len(w))
	header[3] = uint8(len(r) >> 8)
	header[4] = uint8(len(r))

	fmt.Printf("header % x  ", header)

	_, err := bp.c.Write(header)
	if err != nil {
		return nil
	}

	// the slave _would_, according to dangerous prototypes, answer with 0x00
	// now, if either the write or the read count are out of bounds. The bounds
	// are 0-4096 for all BPs known to me.
	// this is seriously bad protocol design. if i would want to deal with
	// different bus pirate versions which support different maximum read/write
	// sizes, I'd either have to settle for the lowest common denominator or
	// wait for the 0x00 to arrive. in fair weather this means that we
	// would have to time out on a non-arriving 0x00 here - on every write then
	// read operation. this bis bonkers and I'm not doing it.

	fmt.Printf("write b % x    ", w)

	_, err = bp.c.Write(w)
	if err != nil {
		return err
	}

	// the ack after the write bytes operation is poorly documented. i believe
	// that the bp will answer bpans_OK if all bytes written have been acked
	// and 0x00 if there was a NACK at some point
	b, err := bp.readByte()
	if err != nil {
		return err
	}

	// we're aliasing all kinds of NACKs into NoSuchDevice - I'm not sure this
	// is a good idea, but at this point I don't care any more.
	if b != bpans_OK {
		return i2cm.NoSuchDevice
	}

	fmt.Printf("  ACK!   ")

	if len(r) > 0 {
		_, err = io.ReadFull(bp.c, r)
		if err != nil {
			return err
		}
	}

	fmt.Printf("read b % x\n", r)

	return nil
}

// only supports 7 bit addressing
func (nsi NonStrictI2C) Transact8x8(addr i2cm.Addr, regaddr uint8, w []byte, r []byte) (nw, nr int, err error) {
	bp := nsi.bp
	if bp.mode != MODE_I2C {
		return 0, 0, notI2CMode
	}

	if addr.GetAddrLen() != 7 {
		return 0, 0, errors.New("bp nonstrict I2C only supports 7 bit addressing")
	}

	fmt.Printf("nonstrict Transact8x8 addr %v regaddr %#02x len(w) %d len(r) %d\n", addr, regaddr, len(w), len(r))

	// we need one byte for the device address
	maxwsize := i2c_RnW_MAXWRITE - 1
	if len(w) > maxwsize {
		return 0, 0, fmt.Errorf("Transact8x8: write of %d bytes requested, maximum of %d supported", len(w), maxwsize)
	}

	maxrsize := i2c_RnW_MAXREAD
	if len(r) > maxrsize {
		return 0, 0, fmt.Errorf("Transact8x8: read of %d bytes requested, maximum of %d supported", len(r), maxrsize)
	}

	// prepend device and register address
	wbuf := make([]byte, 0, len(w)+2)
	wbuf = append(wbuf, uint8(addr.GetBaseAddr())<<1) // write addr
	wbuf = append(wbuf, regaddr)
	wbuf = append(wbuf, w...)

	// the write part of the transaction
	err = nsi.writeThenRead(wbuf, nil)
	if err != nil {
		// actually, we don't know anything about the number of bytes written
		return 0, 0, err
	}

	wbuf = wbuf[0:1]
	wbuf[0] = uint8(addr.GetBaseAddr()<<1) | 1 // read addr

	// the read part of the transaction
	err = nsi.writeThenRead(wbuf, r)
	if err != nil {
		return 0, 0, err
	}

	return len(w), len(r), nil
}
