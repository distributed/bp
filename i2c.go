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
	bpcmd_I2C_BULK_WRITE = 0x10
)

// Start sends a start or repeated start bit.
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
