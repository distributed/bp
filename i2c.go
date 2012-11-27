package bp

import (
	"bytes"
	"fmt"
	"i2cm"
	"io"
)

type BusPirateI2C struct {
	bp *BusPirate
}

func (bp *BusPirate) EnterI2CMode() (BusPirateI2C, error) {
	var bpi2c BusPirateI2C

	if bp.mode != MODE_BITBANG {
		return bpi2c, ModeError("I2C mode can only be entered from raw bitbang mode")
	}

	err := bp.writeByte(0x02)
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

// Start sends a start or repeated start bit.
func (inf BusPirateI2C) Start() error {
	bp := inf.bp
	if bp.mode != MODE_I2C {
		return notI2CMode
	}

	return bp.exchangeByteAndExpect(0x02, 0x01)
}

func (inf *BusPirateI2C) Stop() error {
	bp := inf.bp
	if bp.mode != MODE_I2C {
		return notI2CMode
	}

	return bp.exchangeByteAndExpect(0x03, 0x01)
}

func (inf BusPirateI2C) ReadByte(ack bool) (byte, error) {
	bp := inf.bp
	if bp.mode != MODE_I2C {
		return 0x00, notI2CMode
	}

	b, err := bp.exchangeByte(0x05)
	if err != nil {
		return 0, err
	}

	if ack {
		err = bp.exchangeByteAndExpect(0x06, 0x01)
	} else {
		err = bp.exchangeByteAndExpect(0x07, 0x01)
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
	cmd := byte(0x10 | 0x00)
	if err := bp.exchangeByteAndExpect(cmd, 0x01); err != nil {
		return err
	}

	ackb, err := bp.exchangeByte(b)
	if err != nil {
		return err
	}

	if ackb != 0 {
		return i2cm.NACKReceived
	}

	return nil
}
