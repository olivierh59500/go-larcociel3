// Package demo assembles the French screen's original bitmap and raster layers.
package demo

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	_ "image/png"
	"io"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/olivierh59500/democonstructionkit/scrolling"
	"github.com/olivierh59500/democonstructionkit/sound"
	playback "github.com/olivierh59500/democonstructionkit/sound/ebiten"
	"github.com/olivierh59500/democonstructionkit/sprites"
	"github.com/olivierh59500/go-larcociel3/assets"
	"github.com/olivierh59500/go-larcociel3/internal/source"
)

const Width, Height, FPS = 320, 200, 50

type Game struct {
	clock                             *source.Clock
	images                            []*ebiten.Image
	batch                             *sprites.ImageSlots
	slots                             [96]sprites.ImageSlot
	strips                            [96][4]*ebiten.Image
	stream                            *scrolling.SliceStream
	scroll                            *scrolling.Scrolling
	profile                           [40]int
	background, body, text, landscape *ebiten.Image
	landscapeData, rasterData         []byte
	pixels                            [320 * 100 * 4]byte
	player                            *playback.Player
	visual                            *sound.Stream
	pcm                               [960 * 8]byte
	shader                            *ebiten.Shader
	palette                           [64]float32
	rows                              [200 * 4]float32
	ink                               [200 * 4]float32
	rasterPairs                       []byte
	meterColors                       []byte
	uniforms                          map[string]any
	closed                            bool
}

func resource(name string) ([]byte, error) { return assets.Files.ReadFile("original/" + name) }
func rgb(dst []float32, w uint16) {
	dst[0] = float32(w>>8&7) * 34 / 255
	dst[1] = float32(w>>4&7) * 34 / 255
	dst[2] = float32(w&7) * 34 / 255
	dst[3] = 1
}

func NewGame(mute bool) (_ *Game, err error) {
	g := &Game{}
	defer func() {
		if err != nil {
			g.Close()
		}
	}()
	read := func(name string) []byte {
		if err != nil {
			return nil
		}
		var b []byte
		b, err = resource(name)
		return b
	}
	state, path, program := read("initial-sprites.bin"), read("logo-path.bin"), read("landscape-script.bin")
	if err != nil {
		return nil, err
	}
	g.clock, err = source.NewClock(state, path, program)
	if err != nil {
		return nil, err
	}
	load := func(name string) (*ebiten.Image, error) {
		b, e := resource(name)
		if e != nil {
			return nil, e
		}
		im, _, e := image.Decode(bytes.NewReader(b))
		if e != nil {
			return nil, e
		}
		texture := ebiten.NewImageFromImage(im)
		g.images = append(g.images, texture)
		return texture, nil
	}
	g.background, err = load("background.png")
	if err != nil {
		return nil, err
	}
	if _, err = load("logo.png"); err != nil {
		return nil, err
	}
	if _, err = load("sprite.png"); err != nil {
		return nil, err
	}
	for i := 0; i < 4; i++ {
		if _, err = load(fmt.Sprintf("pillar-%d.png", i)); err != nil {
			return nil, err
		}
	}
	atlas, err := load("font.png")
	if err != nil {
		return nil, err
	}
	for i := range g.strips {
		for col := range g.strips[i] {
			g.strips[i][col] = atlas.SubImage(image.Rect(i*32+col*8, 0, i*32+col*8+8, 16)).(*ebiten.Image)
		}
	}
	message := read("message.txt")
	profile := read("scroll-profile.json")
	g.landscapeData = read("landscape.bin")
	g.rasterData = read("raster-data.bin")
	g.rasterPairs = read("raster-pairs.bin")
	g.meterColors = read("meter-colors.bin")
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(profile, &g.profile); err != nil {
		return nil, err
	}
	tokens := make([]scrolling.SliceToken, 0, len(message))
	for _, c := range message {
		glyph := int(c) - 32
		if glyph < 0 || glyph >= 96 {
			glyph = 0
		}
		tokens = append(tokens, scrolling.SliceToken{Glyph: glyph, Width: 32})
	}
	g.stream, err = scrolling.NewSliceStream(scrolling.SliceStreamConfig{Tokens: tokens, Capacity: 40, SliceWidth: 8, Repeat: true, Initial: scrolling.DNASlice{Glyph: -1}})
	if err != nil {
		return nil, err
	}
	g.scroll, err = scrolling.New(scrolling.Config{GlyphWindow: &scrolling.GlyphWindowConfig{Count: 40, Advance: 8, Glyph: func(slot int) scrolling.Glyph {
		s := g.stream.Slices()[(g.stream.Head()+slot)%40]
		glyph := scrolling.Glyph{Advance: 8, Y: float64(g.profile[slot])}
		if s.Glyph >= 0 {
			glyph.Image = g.strips[s.Glyph][s.Slice]
		}
		return glyph
	}}})
	if err != nil {
		return nil, err
	}
	g.body = ebiten.NewImage(Width, Height)
	g.text = ebiten.NewImage(Width, Height)
	g.landscape = ebiten.NewImage(Width, 100)
	g.batch, err = sprites.NewImageSlots(sprites.ImageSlotsConfig{Images: g.images, MaxSlots: 96})
	if err != nil {
		return nil, err
	}
	pal := read("palette.bin")
	if err != nil {
		return nil, err
	}
	for i := 0; i < 16; i++ {
		rgb(g.palette[i*4:], binary.BigEndian.Uint16(pal[i*2:]))
	}
	rgb(g.palette[4*4:], 0x667)
	rgb(g.palette[5*4:], 0x667)
	rgb(g.palette[6*4:], 0x400)
	rgb(g.palette[12*4:], 0x223)
	rgb(g.palette[14*4:], 0x400)
	g.uniforms = map[string]any{"Palette": g.palette[:], "Rows": g.rows[:], "Ink": g.ink[:]}
	g.shader, err = ebiten.NewShader([]byte(paletteShader))
	if err != nil {
		return nil, err
	}
	music := read("music.ym")
	if err != nil {
		return nil, err
	}
	g.visual, err = sound.Open("music.ym", music, sound.Options{SampleRate: 48000, BlockFrames: 960, Loop: true})
	if err != nil {
		return nil, err
	}
	if !mute {
		g.player, err = playback.Open(nil, "music.ym", music, sound.Options{SampleRate: 48000, BlockFrames: 960, Loop: true})
		if err != nil {
			return nil, err
		}
		g.player.Play()
	}
	return g, nil
}
func (g *Game) Update() error {
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) || inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return ebiten.Termination
	}
	g.clock.Step()
	g.stream.Step(1, nil)
	if _, err := io.ReadFull(g.visual, g.pcm[:]); err != nil {
		return err
	}
	g.body.Clear()
	count := 0
	for row, x := range g.clock.LogoX {
		g.slots[count] = sprites.ImageSlot{Image: 1, Source: image.Rect(0, 31-row, 208, 32-row), X: float64(x), Y: float64(40 - row)}
		count++
	}
	for i := 27; i >= 0; i-- {
		g.slots[count] = sprites.ImageSlot{Image: 2, X: float64(g.clock.SpritesX[i]), Y: float64(g.clock.SpritesY[i])}
		count++
	}
	phase := (g.clock.Tick - 1) % 319
	if phase < 68 {
		g.slots[count] = sprites.ImageSlot{Image: 3 + phase%4, X: float64(phase / 4 * 16), Y: 171}
		count++
	}

	if err := g.batch.SetSlots(g.slots[:count]); err != nil {
		return err
	}
	g.batch.Draw(g.body)
	g.text.Clear()
	g.scroll.Draw(g.text)
	if err := g.scroll.Err(); err != nil {
		return err
	}
	g.clock.LandscapePixels(g.pixels[:], g.landscapeData)
	g.landscape.WritePixels(g.pixels[:])
	var meters [48]uint16
	if regs, ok := g.visual.YMRegisters(); ok {
		for channel := 0; channel < 3; channel++ {
			level := max(0, int(regs[8+channel]&15)-1)
			blank := 15 - level
			for row := blank; row < 16; row++ {
				meters[row*3+channel] = binary.BigEndian.Uint16(g.meterColors[(channel*16+row-blank)*2:])
			}
		}
	}
	for y := 0; y < 200; y++ {
		w := uint16(0x223)
		if y >= 100 && y < 196 {
			w = meters[(y-100)/2]
		}
		if y < 100 {
			off := (y * 4) % len(g.rasterData)
			w = binary.BigEndian.Uint16(g.rasterData[off:])
		}
		rgb(g.rows[y*4:], w)
		foreground := uint16(0x234)
		if y < 100 {
			foreground = binary.BigEndian.Uint16(g.rasterPairs[y*4+2:])
		}
		rgb(g.ink[y*4:], foreground)
	}
	return nil
}
func (g *Game) Draw(dst *ebiten.Image) {
	v := []ebiten.Vertex{{DstX: 0, DstY: 0, SrcX: 0, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1}, {DstX: Width, DstY: 0, SrcX: Width, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1}, {DstX: 0, DstY: Height, SrcX: 0, SrcY: Height, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1}, {DstX: Width, DstY: Height, SrcX: Width, SrcY: Height, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1}}
	dst.DrawTrianglesShader(v, []uint16{0, 1, 2, 1, 3, 2}, g.shader, &ebiten.DrawTrianglesShaderOptions{Images: [4]*ebiten.Image{g.background, g.body, g.text, g.landscape}, Uniforms: g.uniforms, Blend: ebiten.BlendCopy})
}
func (*Game) Layout(int, int) (int, int) { return Width, Height }
func (g *Game) Tick() int                { return g.clock.Tick }
func (g *Game) Close() {
	if g == nil || g.closed {
		return
	}
	g.closed = true
	if g.player != nil {
		g.player.Close()
	}
	if g.visual != nil {
		g.visual.Close()
	}
	if g.scroll != nil {
		g.scroll.Close()
	}
	if g.batch != nil {
		g.batch.Close()
	}
	if g.shader != nil {
		g.shader.Deallocate()
	}
	for _, im := range append(g.images, g.body, g.text, g.landscape) {
		if im != nil {
			im.Deallocate()
		}
	}
}

const paletteShader = `//kage:unit pixels
package main
var Palette [16]vec4
var Rows [200]vec4
var Ink [200]vec4
func Fragment(position vec4,source vec2,color vec4)vec4{
 p:=source-imageSrc0Origin();i:=int(clamp(floor(imageSrc0At(source).r*15+.5),0,15));body:=imageSrc1At(source);text:=imageSrc2At(source)
 if p.y>=100{land:=imageSrc3At(imageSrc0Origin()+vec2(p.x,p.y-100));i=4+int(floor(land.r*15+.5))}
 if body.a>0{b:=int(floor(body.r*15+.5));if b<4{i=(i/4)*4+b}};if text.a>0{i=i-i%2+1}
 if i==0{return Rows[int(p.y)]*color};if p.y<100&&i==1{return Ink[int(p.y)]*color};return Palette[i]*color
}
`
