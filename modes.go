package bp

const (
	MODE_UNKNOWN = iota
	MODE_BITBANG
	MODE_SPI
	MODE_I2C
	MODE_UART
	MODE_1WIRE
	MODE_RAW
)

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
