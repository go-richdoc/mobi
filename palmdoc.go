// Copyright (c) 2026, the go-richdoc/mobi authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package mobi

import "fmt"

// inflate decompresses one PalmDOC record.
//
// The scheme is LZ77 with a literal escape and one trick of its own, and every
// byte of the input says which:
//
//	0x00         one literal byte
//	0x01..0x08   a run of that many literal bytes
//	0x09..0x7F   one literal byte
//	0x80..0xBF   two bytes: a back-reference, 11 bits of distance and 3 of length
//	0xC0..0xFF   a SPACE followed by the byte with its top bit cleared
//
// The last range is the trick: English prose is mostly "space then a letter",
// so the format spends one byte on the commonest pair in the language. It is
// also the reason a MOBI record decompresses to a little over 4096 bytes from
// rather less.
//
// ⛔ EVERY BOUND HERE IS THE FILE'S OWN ARITHMETIC, and all three ways out are
// errors rather than a short answer: a run that claims more literals than
// remain, a distance of zero, and a distance reaching back before the start of
// the output. The first is a truncated record; the last two cannot be produced
// by any compressor and are what a crafted file looks like. A reader that
// clamped them instead would return prose that is not in the book.
func inflate(in []byte, max int) ([]byte, error) {
	out := make([]byte, 0, len(in)*2)
	for i := 0; i < len(in); {
		c := in[i]
		i++
		switch {
		case c == 0 || (c >= 0x09 && c <= 0x7F):
			out = append(out, c)
		case c <= 0x08:
			n := int(c)
			if i+n > len(in) {
				return nil, fmt.Errorf("mobi: a run of %d literal bytes with %d left in the record", n, len(in)-i)
			}
			out = append(out, in[i:i+n]...)
			i += n
		case c <= 0xBF:
			if i >= len(in) {
				return nil, fmt.Errorf("mobi: a back-reference cut off at the end of the record")
			}
			v := int(c&0x3F)<<8 | int(in[i])
			i++
			distance, length := v>>3, (v&7)+3
			if distance == 0 {
				return nil, fmt.Errorf("mobi: a back-reference of distance zero")
			}
			if distance > len(out) {
				return nil, fmt.Errorf("mobi: a back-reference %d bytes back into %d bytes of output", distance, len(out))
			}
			// ⛔ ONE BYTE AT A TIME, and not a slice copy. A back-reference
			// may be SHORTER than its length — distance 1, length 10 means
			// ten copies of the last byte — which is how a run is written,
			// and copy() of a slice taken before the append would read the
			// bytes it is about to write.
			for range length {
				out = append(out, out[len(out)-distance])
			}
		default:
			out = append(out, ' ', c^0x80)
		}
		if len(out) > max {
			return nil, fmt.Errorf("mobi: a record decompressed past the limit of %d bytes", max)
		}
	}
	return out, nil
}
