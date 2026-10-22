package main

import (
	"machine"
	"math"

	pio "github.com/tinygo-org/pio/rp2-pio"
	"github.com/tinygo-org/pio/rp2-pio/piolib"
)

const (
	sampleRate = 22050
	volume     = 8000
	fade       = 64
)

var (
	sine  [256]int16
	steps [13]uint32
)

type synth struct {
	phase uint32
	step  uint32
	amp   int32
	on    bool
}

func (s *synth) fill(buf []uint32) {
	for i := range buf {
		if s.on && s.amp < volume {
			s.amp += fade
		} else if !s.on && s.amp > 0 {
			s.amp -= fade
		}
		s.phase += s.step
		v := uint16(int32(sine[s.phase>>24]) * s.amp >> 15)
		buf[i] = uint32(v)<<16 | uint32(v)
	}
}

func main() {
	for i := range sine {
		sine[i] = int16(32767 * math.Sin(2*math.Pi*float64(i)/256))
	}
	// C4 to C5
	for i := range steps {
		f := 440 * math.Pow(2, float64(i-9)/12)
		steps[i] = uint32(f * (1 << 32) / sampleRate)
	}

	sm, _ := pio.PIO0.ClaimStateMachine()
	i2s, err := piolib.NewI2S(sm, machine.GPIO26, machine.GPIO14)
	if err != nil {
		panic(err)
	}
	// Each I2S bit takes two PIO cycles, so the PIO runs at 64x the sample rate.
	whole, frac, _ := pio.ClkDivFromFrequency(sampleRate*64, machine.CPUFrequency())
	sm.SetClkDiv(whole, frac)

	colPins := []machine.Pin{machine.GPIO5, machine.GPIO6, machine.GPIO7, machine.GPIO8}
	rowPins := []machine.Pin{machine.GPIO9, machine.GPIO10, machine.GPIO11}
	for _, c := range colPins {
		c.Configure(machine.PinConfig{Mode: machine.PinOutput})
		c.Low()
	}
	for _, r := range rowPins {
		r.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
	}

	var s synth
	buf := make([]uint32, 128)
	play := func(n, count int) {
		s.step = steps[n]
		s.on = true
		for i := 0; i < count; i++ {
			s.fill(buf)
			i2s.WriteStereo(buf)
		}
		s.on = false
	}
	for _, n := range []int{0, 4, 7, 12} {
		play(n, 30)
	}

	var keys, next uint16
	note := -1
	col := 0
	colPins[col].High()
	for {
		s.fill(buf)
		i2s.WriteStereo(buf)

		for r, p := range rowPins {
			if p.Get() {
				next |= 1 << (r*4 + col)
			}
		}
		colPins[col].Low()
		col = (col + 1) % len(colPins)
		colPins[col].High()
		if col != 0 {
			continue
		}

		pressed := next &^ keys
		keys, next = next, 0
		switch {
		case pressed != 0:
			note = lowest(pressed)
		case note >= 0 && keys&(1<<note) == 0:
			note = lowest(keys)
		}
		if note >= 0 {
			s.step = steps[note]
		}
		s.on = note >= 0
	}
}

func lowest(k uint16) int {
	for i := 0; i < 12; i++ {
		if k&(1<<i) != 0 {
			return i
		}
	}
	return -1
}
