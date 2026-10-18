// Copyright (c) 2026, the go-richdoc/mobi authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package mobi

import (
	"encoding/binary"
	"testing"
)

// A book is a MOBI assembled field by field, so that a test can say exactly
// what the file claims and then check what the reader makes of it.
//
// ⛔ IT IS NOT A WRITER. There is no MOBI writer in this package and this is
// not the start of one: it exists to put a chosen byte at a chosen offset,
// including bytes no writer would ever produce — a record list that points
// past the end of the file, a title claiming four gigabytes, a compression
// type nobody has heard of. Those are the inputs that matter, and a writer
// would refuse to produce them.
//
// What it CANNOT do is judge the reader's prose: a round trip through a
// fixture only says the two halves agree. That question is answered by a real
// book and an independent rendering of the same text, in corpus_test.go.
type book struct {
	name    string
	typ     string
	creator string
	records [][]byte
}

func newBook() *book {
	return &book{name: "A_Test_Book", typ: "BOOK", creator: "MOBI"}
}

// bytes assembles the Palm database.
func (b *book) bytes() []byte {
	n := len(b.records)
	head := make([]byte, pdbHeaderSize+8*n+2)
	copy(head, b.name)
	copy(head[60:], b.typ)
	copy(head[64:], b.creator)
	binary.BigEndian.PutUint16(head[76:], uint16(n))
	off := len(head)
	for i, r := range b.records {
		binary.BigEndian.PutUint32(head[pdbHeaderSize+8*i:], uint32(off))
		off += len(r)
	}
	out := head
	for _, r := range b.records {
		out = append(out, r...)
	}
	return out
}

// header0 is record 0: the PalmDOC header, the MOBI header, an optional EXTH
// block and the title.
type header0 struct {
	compression uint16
	textLength  uint32
	textRecords uint16
	recordSize  uint16
	encryption  uint16
	mobiType    uint32
	encoding    uint32
	firstImage  uint32
	extraFlags  uint32
	huffOffset  uint32
	huffCount   uint32
	title       string
	exth        []exthRecord
	// headerLength overrides the length written in the MOBI header, for the
	// tests that ask what happens when a field is not there at all.
	headerLength uint32
}

type exthRecord struct {
	typ  uint32
	data string
}

// the MOBI header this builder writes is 264 bytes, which is what Calibre
// writes and long enough to hold every field this reader reads.
const builtHeaderLength = 264

func (h header0) bytes() []byte {
	hl := h.headerLength
	if hl == 0 {
		hl = builtHeaderLength
	}
	r0 := make([]byte, offIdentifier+int(hl))
	binary.BigEndian.PutUint16(r0[offCompression:], h.compression)
	binary.BigEndian.PutUint32(r0[offTextLength:], h.textLength)
	binary.BigEndian.PutUint16(r0[offRecordCount:], h.textRecords)
	binary.BigEndian.PutUint16(r0[offRecordSize:], h.recordSize)
	binary.BigEndian.PutUint16(r0[offEncryption:], h.encryption)
	copy(r0[offIdentifier:], "MOBI")
	binary.BigEndian.PutUint32(r0[offHeaderLength:], hl)
	put := func(off int, v uint32) {
		if off+4 <= len(r0) {
			binary.BigEndian.PutUint32(r0[off:], v)
		}
	}
	put(offMobiType, h.mobiType)
	put(offTextEncoding, h.encoding)
	put(offFirstImage, h.firstImage)
	put(offExtraFlags, h.extraFlags)
	put(offHuffOffset, h.huffOffset)
	put(offHuffCount, h.huffCount)

	if len(h.exth) > 0 {
		put(offEXTHFlags, 0x40)
		body := []byte{}
		for _, e := range h.exth {
			rec := make([]byte, 8+len(e.data))
			binary.BigEndian.PutUint32(rec, e.typ)
			binary.BigEndian.PutUint32(rec[4:], uint32(8+len(e.data)))
			copy(rec[8:], e.data)
			body = append(body, rec...)
		}
		exth := make([]byte, 12)
		copy(exth, "EXTH")
		binary.BigEndian.PutUint32(exth[4:], uint32(12+len(body)))
		binary.BigEndian.PutUint32(exth[8:], uint32(len(h.exth)))
		r0 = append(r0, append(exth, body...)...)
	}
	if h.title != "" {
		// ⛔ Through the same bounded writer as every other field, so that a
		// header too short to hold the title's offset-and-length PAIR builds
		// rather than panicking. Those are the headers worth testing: 72
		// bytes covers the offset at 84 and not the length at 88.
		put(offFullNameOffset, uint32(len(r0)))
		put(offFullNameLength, uint32(len(h.title)))
		r0 = append(r0, h.title...)
	}
	return r0
}

// plainBook is the shortest useful whole book: uncompressed text in one
// record, and nothing else.
func plainBook(t *testing.T, text string, h header0) []byte {
	t.Helper()
	h.compression = compressionNone
	h.textLength = uint32(len(text))
	h.textRecords = 1
	h.recordSize = 4096
	h.encoding = encodingUTF8
	if h.mobiType == 0 {
		h.mobiType = mobiTypeBook
	}
	b := newBook()
	b.records = [][]byte{h.bytes(), []byte(text)}
	return b.bytes()
}
