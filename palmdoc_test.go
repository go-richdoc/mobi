// Copyright (c) 2026, the go-richdoc/mobi authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package mobi

import (
	"strings"
	"testing"
)

// ⛔ THESE BYTES ARE TYPED FROM THE FORMAT'S OWN DESCRIPTION, not produced by a
// compressor of mine. A fixture written by a compressor in this package would
// make the two halves of one misreading agree with each other, and the test
// would pass on prose nobody can read.
func TestEachKindOfPalmDocByte(t *testing.T) {
	for _, c := range []struct {
		name string
		in   []byte
		want string
	}{
		{"a literal byte", []byte{'a', 'b', 'c'}, "abc"},
		{"zero is a literal too", []byte{0x00}, "\x00"},
		{
			// 0x01..0x08 says "the next n bytes are literal", which is how a
			// compressor emits a byte that would otherwise be a marker.
			"a run of literals", []byte{0x03, 0xC1, 0x82, 0x01, 'z'}, "\xc1\x82\x01z",
		},
		{
			// 0xC0..0xFF is a space and the byte with its top bit cleared:
			// the format spends one byte on the commonest pair in English.
			"space and a letter", []byte{0xE8, 0xE5, 0xEC, 0xEC, 0xEF}, " h e l l o",
		},
		{
			// 0x80..0xBF: six bits of the first byte and eight of the second
			// make fourteen, of which eleven are the distance and three the
			// length less three. "abcdefgh" then distance 8, length 3.
			"a back-reference", []byte{'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 0x80, 0x40}, "abcdefghabc",
		},
		{
			// Distance 1, length 10: ten copies of one byte, which is how a
			// run of the same character is written. A copy that took a slice
			// before appending would produce garbage here.
			"a back-reference shorter than its length", []byte{'x', 0x80, 0x0F}, "xxxxxxxxxxx",
		},
		{"nothing at all", []byte{}, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := inflate(c.in, 1<<20)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestPalmDocRefusals(t *testing.T) {
	for _, c := range []struct {
		name string
		in   []byte
		max  int
		says string
	}{
		{"a literal run past the end of the record", []byte{0x05, 'a', 'b'}, 1 << 20, "run of 5 literal bytes"},
		{"a back-reference cut off", []byte{'a', 0x80}, 1 << 20, "cut off"},
		{"a distance of zero", []byte{'a', 0x80, 0x00}, 1 << 20, "distance zero"},
		{"a distance before the start", []byte{'a', 0x80, 0x40}, 1 << 20, "bytes back into"},
		{"more output than the limit", []byte{'a', 'b', 'c', 'd'}, 3, "past the limit"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := inflate(c.in, c.max)
			if err == nil {
				t.Fatal("it was accepted")
			}
			if !strings.Contains(err.Error(), c.says) {
				t.Errorf("the refusal says %q, which does not mention %q", err, c.says)
			}
		})
	}
}
