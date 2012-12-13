// Copyright 2012 Michael Meier. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package bp

import (
	"errors"
	"fmt"
)

const (
	MODE_CLOSED = iota
	MODE_UNKNOWN
	MODE_BITBANG
	MODE_SPI
	MODE_I2C
	MODE_UART
	MODE_1WIRE
	MODE_RAW
)

var modestrings = map[int]string{MODE_CLOSED: "closed",
	MODE_UNKNOWN: "unknown",
	MODE_BITBANG: "bitbang",
	MODE_SPI:     "SPI",
	MODE_I2C:     "I2C",
	MODE_UART:    "UART",
	MODE_1WIRE:   "1Wire",
	MODE_RAW:     "raw",
}

type ModeError string

func (me ModeError) Error() string {
	return string(me)
}

func (bp *BusPirate) clearMode() {
	bp.mode = MODE_UNKNOWN
	bp.modeversion = 0
}

// returns the active mode and the mode's version
func (bp *BusPirate) GetMode() (int, int) {
	return bp.mode, bp.modeversion
}

func (bp *BusPirate) expectMode(mode int) error {
	if bp.mode == MODE_CLOSED {
		return errors.New("BusPirate: connection not open")
	} else if bp.mode == MODE_UNKNOWN {
		return errors.New("BusPirate: mode not known (did you forget to check for a communication error?)")
	}

	if mode != bp.mode {
		expname, expok := modestrings[mode]
		actname, actok := modestrings[bp.mode]
		var expstring, actstring string

		if expok {
			expstring = expname + " mode"
		} else {
			expstring = fmt.Sprintf("mode %d", bp.mode)
		}

		if actok {
			actstring = actname + " mode"
		} else {
			actstring = fmt.Sprintf("mode %d", bp.mode)
		}

		return fmt.Errorf("BusPirate: need to be in %s, currently in %s", expstring, actstring)
	}
	return nil
}
