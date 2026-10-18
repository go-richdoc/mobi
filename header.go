// Copyright (c) 2026, the go-richdoc/mobi authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package mobi

import (
	"encoding/binary"
	"fmt"
)

// the fields of record 0 this reader uses, at their offsets from the start of
// the record. The PalmDOC part is the first sixteen bytes; the MOBI header
// begins at 16 and says how long it is, and everything past that length is
// absent rather than zero.
const (
	offCompression    = 0
	offTextLength     = 4
	offRecordCount    = 8
	offRecordSize     = 10
	offEncryption     = 12
	offIdentifier     = 16
	offHeaderLength   = 20
	offMobiType       = 24
	offTextEncoding   = 28
	offFullNameOffset = 84
	offFullNameLength = 88
	offFirstImage     = 108
	offHuffOffset     = 112
	offHuffCount      = 116
	offEXTHFlags      = 128
	offExtraFlags     = 240
)

const (
	compressionNone     = 1
	compressionPalmDoc  = 2
	compressionHuffCDIC = 17480

	mobiTypeBook = 2
	mobiTypeKF8  = 248

	encodingCP1252 = 1252
	encodingUTF8   = 65001

	absent = 0xFFFFFFFF
)

// a header is what record 0 says about the book.
type header struct {
	compression uint16
	textLength  uint32
	textRecords int
	recordSize  uint16
	encryption  uint16

	mobiType   uint32
	encoding   uint32
	fullName   string
	firstImage uint32
	extraFlags uint16
	exth       map[uint32][]string
}

// readHeader reads record 0 and refuses, by name, the three kinds of MOBI this
// package does not read.
func readHeader(r0 []byte) (*header, error) {
	if len(r0) < 20+4 {
		return nil, fmt.Errorf("%w: record 0 is %d bytes, too short for a MOBI header", ErrFormat, len(r0))
	}
	if string(r0[offIdentifier:offIdentifier+4]) != "MOBI" {
		return nil, fmt.Errorf("%w: record 0 says %q where every MOBI says MOBI",
			ErrFormat, r0[offIdentifier:offIdentifier+4])
	}
	// ⛔ The header's own length is the bound on reading the header: a field
	// beyond it is ABSENT, not zero, and the difference matters for
	// firstImage, where zero is a valid record number and absence means the
	// book has no pictures at all.
	hl := int(binary.BigEndian.Uint32(r0[offHeaderLength:]))
	end := min(offIdentifier+hl, len(r0))

	h := &header{
		compression: binary.BigEndian.Uint16(r0[offCompression:]),
		textLength:  binary.BigEndian.Uint32(r0[offTextLength:]),
		textRecords: int(binary.BigEndian.Uint16(r0[offRecordCount:])),
		recordSize:  binary.BigEndian.Uint16(r0[offRecordSize:]),
		encryption:  binary.BigEndian.Uint16(r0[offEncryption:]),
		mobiType:    mobiTypeBook,
		encoding:    encodingCP1252,
		firstImage:  absent,
	}
	if v, ok := u32(r0, offMobiType, end); ok {
		h.mobiType = v
	}
	if v, ok := u32(r0, offTextEncoding, end); ok {
		h.encoding = v
	}
	if v, ok := u32(r0, offFirstImage, end); ok {
		h.firstImage = v
	}
	if v, ok := u32(r0, offExtraFlags, end); ok {
		// Only the low sixteen bits are used, one bit per kind of trailing
		// entry appended to each text record.
		h.extraFlags = uint16(v)
	}
	h.fullName = fullNameIn(r0, end)

	switch {
	case h.encryption != 0:
		return nil, fmt.Errorf("mobi: %q is DRM'd (encryption type %d), and the text is encrypted to a device key: there is nothing here to read",
			h.fullName, h.encryption)
	case h.compression == compressionHuffCDIC:
		off, _ := u32(r0, offHuffOffset, end)
		n, _ := u32(r0, offHuffCount, end)
		return nil, fmt.Errorf("mobi: %q is HUFF/CDIC-compressed (%d records from %d), which this reader does not decode",
			h.fullName, n, off)
	case h.compression != compressionNone && h.compression != compressionPalmDoc:
		return nil, fmt.Errorf("mobi: compression type %d, which is neither none (1) nor PalmDOC (2)", h.compression)
	case h.mobiType == mobiTypeKF8:
		return nil, fmt.Errorf("mobi: %q is a KF8 (AZW3) book: its text is a skeleton with fragments woven into it, not one HTML document, and that is a different reader",
			h.fullName)
	}
	if h.textRecords == 0 {
		return nil, fmt.Errorf("mobi: %q says it has no text records", h.fullName)
	}
	if h.recordSize == 0 {
		return nil, fmt.Errorf("mobi: %q says its text records are zero bytes", h.fullName)
	}
	h.exth = readEXTH(r0, end)
	return h, nil
}

// u32 reads a four-byte field, reporting whether it is inside the header at
// all.
func u32(b []byte, off, end int) (uint32, bool) {
	if off+4 > end || off+4 > len(b) {
		return 0, false
	}
	return binary.BigEndian.Uint32(b[off:]), true
}

// fullNameIn is the book's title, which is NOT in the MOBI header: the header
// holds an offset and a length pointing elsewhere in record 0, past the EXTH
// block.
// ⛔ A FIELD THE HEADER DOES NOT REACH READS AS ZERO HERE, and that is the
// answer rather than a case of its own: a zero offset and a zero length give
// the empty slice, which is the empty title an absent field means. This had
// two `if !ok { return "" }` guards in front of it, and a mutation sweep
// survived on both — they are the same answer by another line, and a branch
// nothing can distinguish is a branch no test can hold.
func fullNameIn(r0 []byte, end int) string {
	off, _ := u32(r0, offFullNameOffset, end)
	n, _ := u32(r0, offFullNameLength, end)
	// ⛔ Checked against the RECORD, not against the header: the name lives
	// outside the header by design. Both of these are numbers in the file, and
	// a title claiming four gigabytes is a slice out of range.
	if int64(off)+int64(n) > int64(len(r0)) {
		return ""
	}
	return string(trimNul(r0[off : off+n]))
}

// readEXTH reads the metadata block that follows the MOBI header, keeping
// every value for a type rather than the last: a book has one title and seven
// subjects.
func readEXTH(r0 []byte, end int) map[uint32][]string {
	flags, ok := u32(r0, offEXTHFlags, end)
	if !ok || flags&0x40 == 0 {
		return nil
	}
	// It begins where the MOBI header ends.
	p := end
	if p+12 > len(r0) || string(r0[p:p+4]) != "EXTH" {
		return nil
	}
	count := int(binary.BigEndian.Uint32(r0[p+8:]))
	out := map[uint32][]string{}
	p += 12
	for range count {
		if p+8 > len(r0) {
			return out
		}
		typ := binary.BigEndian.Uint32(r0[p:])
		length := int(binary.BigEndian.Uint32(r0[p+4:]))
		// ⛔ A length that does not count its own eight bytes, or runs past
		// the record, ends the walk INSTEAD of being skipped: the records are
		// a chain, so one bad length means every offset after it is
		// guesswork. What has been read is kept, because metadata is a bonus
		// and the book is the point.
		if length < 8 || p+length > len(r0) {
			return out
		}
		out[typ] = append(out[typ], string(trimNul(r0[p+8:p+length])))
		p += length
	}
	return out
}

// first is the first value for an EXTH type, or "".
func (h *header) first(typ uint32) string {
	if v := h.exth[typ]; len(v) > 0 {
		return v[0]
	}
	return ""
}
