// Copyright (c) 2026, the go-richdoc/mobi authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package mobi

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// bookText assembles the book's HTML out of the text records.
//
// The book is ONE document cut into 4096-byte pieces and each piece compressed
// on its own, so this is a concatenation — and a tag may be split across two
// records, which is why nothing may be parsed until every record is in hand.
func bookText(c *container, h *header, maxBytes int64) ([]byte, error) {
	out := make([]byte, 0, 64<<10)
	for i := 1; i <= h.textRecords; i++ {
		rec := c.record(i)
		if rec == nil {
			return nil, fmt.Errorf("mobi: the book says it has %d text records and the file holds %d",
				h.textRecords, c.count()-1)
		}
		body, err := stripTrailing(rec, h.extraFlags)
		if err != nil {
			return nil, fmt.Errorf("mobi: text record %d: %w", i, err)
		}
		remaining := maxBytes - int64(len(out))
		if remaining <= 0 {
			return nil, fmt.Errorf("mobi: the text passed the limit of %d bytes at record %d", maxBytes, i)
		}
		if h.compression == compressionNone {
			if int64(len(body)) > remaining {
				return nil, fmt.Errorf("mobi: the text passed the limit of %d bytes at record %d", maxBytes, i)
			}
			out = append(out, body...)
			continue
		}
		got, err := inflate(body, int(min(remaining, int64(maxInt))))
		if err != nil {
			return nil, fmt.Errorf("mobi: text record %d: %w", i, err)
		}
		out = append(out, got...)
	}
	// ⛔ The records are PADDED and the header says where the book stops: the
	// last block is a whole block, and whatever follows the end of the text
	// in it is not text. A reader that keeps it ends the book with bytes the
	// producer left behind.
	if int64(len(out)) < int64(h.textLength) {
		return nil, fmt.Errorf("mobi: the book claims %d bytes of text and its records hold %d",
			h.textLength, len(out))
	}
	return out[:h.textLength], nil
}

// maxInt is the largest int on this platform, which on js/wasm and 32-bit
// targets is smaller than a MaxTextBytes a caller may legitimately ask for.
const maxInt = int(^uint(0) >> 1)

// stripTrailing removes the entries appended to a text record AFTER
// compression, and returns the compressed text alone.
//
// ⛔ THESE BYTES ARE NOT TEXT AND THEY ARE NOT COMPRESSED. They are indexing
// data and, for a multibyte character split across two records, the overlap.
// Feeding them to the decompressor produces plausible rubbish at the end of
// every record — not an error, just wrong prose, which is the worst kind of
// failure a reader has.
//
// Each set bit above the lowest means one entry is present, and each entry
// ends with its own size written BACKWARDS as a variable-width integer that
// counts itself. The lowest bit is different: it is the multibyte overlap,
// whose count is two bits of the last byte.
func stripTrailing(rec []byte, flags uint16) ([]byte, error) {
	n := 0
	for f := flags >> 1; f != 0; f >>= 1 {
		if f&1 == 0 {
			continue
		}
		size, err := backwardVarint(rec[:len(rec)-n])
		if err != nil {
			return nil, err
		}
		n += size
		if n > len(rec) {
			return nil, fmt.Errorf("the trailing entries claim %d bytes of a %d-byte record", n, len(rec))
		}
	}
	if flags&1 != 0 {
		if len(rec)-n < 1 {
			return nil, fmt.Errorf("a multibyte overlap entry in a record with no bytes left")
		}
		n += int(rec[len(rec)-n-1]&0x3) + 1
		if n > len(rec) {
			return nil, fmt.Errorf("the trailing entries claim %d bytes of a %d-byte record", n, len(rec))
		}
	}
	return rec[:len(rec)-n], nil
}

// backwardVarint reads the size written at the END of a trailing entry,
// working back from the last byte. The high bit marks the last byte read,
// which is the number's most significant; seven bits come from each byte; and
// the size it gives INCLUDES the bytes it is written in.
func backwardVarint(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, fmt.Errorf("a trailing entry size at the end of an empty record")
	}
	size, shift := 0, 0
	for i := len(b) - 1; ; i-- {
		v := b[i]
		size |= int(v&0x7F) << shift
		shift += 7
		// ⛔ Four bytes at most. Without the shift bound a record of 0x00
		// bytes walks to the start of the record and shifts an int by 7 a
		// thousand times, which is not an error anywhere: it just quietly
		// gives zero, and a zero-size entry strips nothing.
		if v&0x80 != 0 || shift >= 28 || i == 0 {
			break
		}
	}
	if size == 0 {
		return 0, fmt.Errorf("a trailing entry whose size is zero")
	}
	return size, nil
}

// decodeText turns the book's bytes into UTF-8, which is what every reader
// downstream of this expects.
func decodeText(b []byte, encoding uint32) (string, error) {
	switch encoding {
	case encodingUTF8:
		// ⛔ Not validated and not repaired. A Kindle book that says UTF-8 and
		// holds a bad byte somewhere is still a book, the HTML parser
		// replaces what it cannot read, and refusing the whole file over one
		// byte would lose the other million.
		return string(b), nil
	case encodingCP1252:
		return cp1252(b), nil
	default:
		return "", fmt.Errorf("mobi: text encoding %d, and a MOBI is either CP1252 (1252) or UTF-8 (65001)", encoding)
	}
}

// cp1252 decodes Windows-1252, which is Latin-1 with twenty-seven characters
// in the gap Latin-1 leaves empty — the curly quotes, the dashes, the euro
// sign and the ellipsis that make up most of the punctuation in an English
// book written on Windows.
//
// ⛔ A TABLE RATHER THAN A DEPENDENCY, and rather than treating the bytes as
// Latin-1. Taken as Latin-1, 0x92 — the apostrophe in "don't" as Word writes
// it — becomes U+0092, an invisible control character: the text looks like
// "dont" with a hole in it, in every contraction in the book.
func cp1252(b []byte) string {
	var sb strings.Builder
	sb.Grow(len(b))
	for _, c := range b {
		switch {
		case c < 0x80:
			sb.WriteByte(c)
		case c >= 0xA0:
			sb.WriteRune(rune(c)) // Latin-1 and Unicode agree from here up
		default:
			r := cp1252High[c-0x80]
			if r == 0 {
				// The five unassigned ones. U+FFFD says "something was here
				// and it is not representable", which is true.
				r = utf8.RuneError
			}
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// cp1252High is 0x80..0x9F, the only range where CP1252 and Latin-1 differ.
// Zero means the byte is unassigned in the encoding.
var cp1252High = [32]rune{
	'€', 0, '‚', 'ƒ', '„', '…', '†', '‡',
	'ˆ', '‰', 'Š', '‹', 'Œ', 0, 'Ž', 0,
	0, '‘', '’', '“', '”', '•', '–', '—',
	'˜', '™', 'š', '›', 'œ', 0, 'ž', 'Ÿ',
}
