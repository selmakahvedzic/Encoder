package structex

import (
	"encoding/binary"
	"fmt"
	"testing"
)

const (
	BufferSize = 512
)

type testWriter struct {
	bytes  [BufferSize]byte
	nbytes int
}

func TestNas5GSUpdateType(t *testing.T) {
	type T struct {
		A uint8
		B uint8
	}

	s := struct {
		Count uint8 `countOf:"Ts"`
		Size  uint8 `sizeOf:"Ts"`
		Ts    [2]T
	}{
		Count: 0x00,
		Size:  0x00,
		Ts: [2]T{
			{A: 0x01, B: 0x02},
			{A: 0x03, B: 0x04},
		},
	}

	packAndTest(t, s, func(t *testing.T, tw *testWriter) {
		if tw.getByte(0) != 2 {
			t.Errorf("Invalid countOf: Expected: %d Actual: %d", 2, tw.getByte(0))
		}
		if tw.getByte(1) != 4 {
			t.Errorf("Invalid sizeOf: Expected: %d Actual: %d", 4, tw.getByte(1))
		}

		expected := []uint8{0x01, 0x02, 0x03}
		actual := []uint8{tw.getByte(2), tw.getByte(3), tw.getByte(4)}

		for i := 0; i < 3; i++ {
			if expected[i] != actual[i] {
				t.Errorf("Invalid array pack: Index: %d Expected: %#02x Actual: %#02x", i, expected[i], actual[i])
			}
		}
	})
}
