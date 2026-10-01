package source

import (
	"encoding/json"
	"github.com/olivierh59500/go-larcociel3/assets"
	"os"
	"reflect"
	"testing"
)

func TestSpritesMatchNativeCheckpoint(t *testing.T) {
	read := func(name string) []byte {
		b, e := assets.Files.ReadFile("original/" + name)
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	c, e := NewClock(read("initial-sprites.bin"), read("logo-path.bin"), read("landscape-script.bin"))
	if e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile("testdata/native-sprites-149.json")
	if e != nil {
		t.Fatal(e)
	}
	var want struct {
		Steps                  int
		X, Y                   [32]int16
		Velocity, Acceleration [2]int16
	}
	if e = json.Unmarshal(b, &want); e != nil {
		t.Fatal(e)
	}
	for range want.Steps {
		c.Step()
	}
	if !reflect.DeepEqual(c.SpritesX, want.X) || !reflect.DeepEqual(c.SpritesY, want.Y) || [2]int16{c.VX, c.VY} != want.Velocity || [2]int16{c.AX, c.AY} != want.Acceleration {
		t.Fatalf("native sprite trail differs: x=%d y=%d velocity=(%d,%d)", c.X, c.Y, c.VX, c.VY)
	}
}
