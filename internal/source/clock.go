// Package source retains the French screen's motion and landscape programs.
package source

import (
	"encoding/binary"
	"fmt"
)

type Clock struct {
	Tick, PathCursor, LandscapeOffset, SceneCounter, SceneCursor int
	X, Y, VX, VY, AX, AY                                         int16
	SpritesX, SpritesY                                           [32]int16
	LogoX                                                        [32]int16
	Path, LandscapeProgram                                       []byte
}

func NewClock(sprites, path, program []byte) (*Clock, error) {
	if len(sprites) != 136 || len(path) < 4 || len(program) < 16 {
		return nil, fmt.Errorf("source: incomplete French motion tables")
	}
	w := func(at int) int16 { return int16(binary.BigEndian.Uint16(sprites[at:])) }
	c := &Clock{X: w(0), Y: w(64), VX: w(128), VY: w(130), AX: w(132), AY: w(134), Path: path, LandscapeProgram: program, LandscapeOffset: 4000, SceneCounter: 1000}
	for i := 0; i < 32; i++ {
		c.SpritesX[i] = w(i * 2)
		c.SpritesY[i] = w(64 + i*2)
		c.LogoX[i] = 64
	}
	return c, nil
}
func (c *Clock) Step() {
	c.Tick++
	copy(c.SpritesX[1:29], c.SpritesX[:28])
	copy(c.SpritesY[1:29], c.SpritesY[:28])
	c.X += c.VX
	c.Y += c.VY
	c.SpritesX[0] = c.X
	c.SpritesY[0] = c.Y
	if c.Tick%2 == 1 {
		c.VX += c.AX
		c.VY += c.AY
		if c.VX == 11 || c.VX == -11 {
			c.AX = -c.AX
		}
		if c.VY == 6 || c.VY == -6 {
			c.AY = -c.AY
		}
	}
	copy(c.LogoX[1:], c.LogoX[:31])
	if c.PathCursor+2 > len(c.Path) || int16(binary.BigEndian.Uint16(c.Path[c.PathCursor:])) < 0 {
		c.PathCursor = 0
	}
	c.LogoX[0] = int16(binary.BigEndian.Uint16(c.Path[c.PathCursor:]))
	c.PathCursor += 2
	c.SceneCounter--
	if c.SceneCounter < 0 {
		c.SceneCursor += 8
		if c.SceneCursor+8 > len(c.LandscapeProgram) || binary.BigEndian.Uint32(c.LandscapeProgram[c.SceneCursor:]) == 0xffff {
			c.SceneCursor = 0
		}
		c.SceneCounter = int(binary.BigEndian.Uint32(c.LandscapeProgram[c.SceneCursor:]))
	}
	step := int(int32(binary.BigEndian.Uint32(c.LandscapeProgram[c.SceneCursor+4:])))
	c.LandscapeOffset = (c.LandscapeOffset + step + 100000) % 100000
}
func (c *Clock) LandscapePixels(dst, data []byte) {
	for y := 0; y < 100; y++ {
		for x := 0; x < 320; x++ {
			off := (c.LandscapeOffset + y*40 + x/16*2) % len(data)
			w := uint16(data[off])<<8 | uint16(data[(off+1)%len(data)])
			v := byte(w >> uint(15-x%16) & 1)
			at := (y*320 + x) * 4
			dst[at] = v * 136
			dst[at+1] = 0
			dst[at+2] = 0
			dst[at+3] = 255
		}
	}
}
