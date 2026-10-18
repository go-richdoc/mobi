// Copyright (c) 2026, the go-richdoc/mobi authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package mobi

import (
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

func TestWhatRecordZeroSays(t *testing.T) {
	h := header0{
		compression: compressionPalmDoc,
		textLength:  1234,
		textRecords: 7,
		recordSize:  4096,
		encoding:    encodingUTF8,
		mobiType:    mobiTypeBook,
		firstImage:  40,
		extraFlags:  0x3,
		title:       "The Title In The Header",
		exth: []exthRecord{
			{100, "An Author"},
			{100, "Another Author"},
			{105, "Fiction"},
			{503, "The Updated Title"},
		},
	}
	got, err := readHeader(h.bytes())
	if err != nil {
		t.Fatal(err)
	}
	if got.compression != compressionPalmDoc || got.textLength != 1234 || got.textRecords != 7 ||
		got.recordSize != 4096 || got.encoding != encodingUTF8 || got.firstImage != 40 || got.extraFlags != 3 {
		t.Errorf("the header came back as %+v", got)
	}
	if got.fullName != "The Title In The Header" {
		t.Errorf("the title is %q", got.fullName)
	}
	// ⛔ EVERY value for a type, not the last: a book has one title and seven
	// subjects, and a map of one string a type would keep "Another Author"
	// and lose the first.
	if len(got.exth[100]) != 2 {
		t.Errorf("%d authors, want two: %q", len(got.exth[100]), got.exth[100])
	}
	if got.first(503) != "The Updated Title" {
		t.Errorf("EXTH 503 is %q", got.first(503))
	}
	if got.first(999) != "" {
		t.Errorf("a type the book does not carry gave %q", got.first(999))
	}
}

// ⛔ A HEADER TOO SHORT TO HOLD A FIELD leaves that field ABSENT, which is not
// the same as zero. Zero is a valid first-image record; absence means the book
// has no pictures.
//
// ⛔ The first version of this test asserted that a 24-byte header has no
// encoding and no type either, and the code was right and the test was wrong:
// the header's length COUNTS FROM THE IDENTIFIER at offset 16, so 24 bytes
// reach offset 40 and the type (24) and encoding (28) are inside it. A minimal
// MOBI header is exactly those six fields. Only the fields past offset 40 —
// the first-image index at 108, the EXTH flags at 128, the extra-data flags at
// 240 — are the ones a short header does not have.
func TestAHeaderTooShortForItsLaterFields(t *testing.T) {
	h := header0{
		compression:  compressionNone,
		textLength:   4,
		textRecords:  1,
		recordSize:   4096,
		mobiType:     mobiTypeBook,
		encoding:     encodingUTF8,
		headerLength: 24,
	}
	got, err := readHeader(h.bytes())
	if err != nil {
		t.Fatal(err)
	}
	if got.firstImage != absent {
		t.Errorf("the first image index is %d, and the header does not reach that field", got.firstImage)
	}
	if got.encoding != encodingUTF8 || got.mobiType != mobiTypeBook {
		t.Errorf("the encoding is %d and the type %d, and a 24-byte header holds both", got.encoding, got.mobiType)
	}
	if got.fullName != "" || got.exth != nil {
		t.Errorf("a title %q and EXTH %v came out of a header that holds neither", got.fullName, got.exth)
	}
	if got.extraFlags != 0 {
		t.Errorf("extra-data flags %#x came out of a header that stops long before them", got.extraFlags)
	}
}

// ⛔ A HEADER SHORTER THAN ITS OWN MANDATORY FIELDS cannot be written by any
// producer — the format's minimum is 24 — so this is a crafted file, and what
// it gets is the defaults rather than a refusal: CP1252 and "a book", which
// are what a MOBI that does not say means.
func TestAHeaderShorterThanTheFormatAllows(t *testing.T) {
	h := header0{
		compression:  compressionNone,
		textLength:   4,
		textRecords:  1,
		recordSize:   4096,
		mobiType:     mobiTypeKF8, // in the bytes, and outside the header
		encoding:     999,
		headerLength: 8,
	}
	got, err := readHeader(h.bytes())
	if err != nil {
		t.Fatal(err)
	}
	if got.encoding != encodingCP1252 {
		t.Errorf("the encoding is %d; a MOBI that does not say is CP1252", got.encoding)
	}
	// ⛔ And the KF8 refusal does NOT fire, because the type is outside the
	// header: a field the header does not cover is not a claim the file makes.
	if got.mobiType != mobiTypeBook {
		t.Errorf("the type is %d; a MOBI that does not say is a book", got.mobiType)
	}
}

func TestTheMobiFilesThisReaderSaysNoTo(t *testing.T) {
	for _, c := range []struct {
		name   string
		h      header0
		says   string
		format bool
	}{
		{
			"a DRM'd book",
			header0{compression: compressionPalmDoc, textRecords: 1, recordSize: 4096, encryption: 2, title: "Bought From A Shop"},
			"DRM", false,
		},
		{
			"HUFF/CDIC compression",
			header0{compression: compressionHuffCDIC, textRecords: 1, recordSize: 4096, huffOffset: 300, huffCount: 2, title: "A Commercial Title"},
			"HUFF/CDIC", false,
		},
		{
			"a compression nobody has heard of",
			header0{compression: 999, textRecords: 1, recordSize: 4096},
			"compression type 999", false,
		},
		{
			"a KF8 book",
			header0{compression: compressionPalmDoc, textRecords: 1, recordSize: 4096, mobiType: mobiTypeKF8, title: "An AZW3"},
			"KF8", false,
		},
		{
			"a book with no text records",
			header0{compression: compressionNone, recordSize: 4096},
			"no text records", false,
		},
		{
			"a book whose text records are zero bytes",
			header0{compression: compressionNone, textRecords: 1},
			"zero bytes", false,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := readHeader(c.h.bytes())
			if err == nil {
				t.Fatal("it was accepted")
			}
			if !strings.Contains(err.Error(), c.says) {
				t.Errorf("the refusal says %q, which does not mention %q", err, c.says)
			}
			// ⛔ NONE of these is ErrFormat. Every one of them IS a Mobipocket
			// file; three of the four are not even defective. A caller
			// sniffing formats must be told "mine, and here is why not"
			// rather than "not mine".
			if errors.Is(err, ErrFormat) {
				t.Errorf("%q came back as ErrFormat, which says the file is not a MOBI at all", err)
			}
		})
	}
}

func TestWhatIsNotAMobiHeader(t *testing.T) {
	good := header0{compression: compressionNone, textRecords: 1, recordSize: 4096}.bytes()

	notMobi := append([]byte{}, good...)
	copy(notMobi[offIdentifier:], "TEXt")

	for _, c := range []struct {
		name string
		in   []byte
		says string
	}{
		{"a record too short to hold the header", good[:20], "too short"},
		{"a record that does not say MOBI", notMobi, `says "TEXt"`},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := readHeader(c.in)
			if err == nil {
				t.Fatal("it was accepted")
			}
			if !strings.Contains(err.Error(), c.says) {
				t.Errorf("the refusal says %q, which does not mention %q", err, c.says)
			}
			if !errors.Is(err, ErrFormat) {
				t.Errorf("%q is not ErrFormat, and these two really are not MOBI files", err)
			}
		})
	}
}

// ⛔ A TITLE IS A LENGTH AND AN OFFSET, both of them in the file, and the title
// lives OUTSIDE the header it is named in. One claiming to run past the end of
// record 0 is a slice out of range, and a book with a corrupt title field is
// still a readable book, so it comes back empty rather than refused.
func TestATitleThatPointsOutOfTheRecord(t *testing.T) {
	h := header0{compression: compressionNone, textRecords: 1, recordSize: 4096, title: "Real Title"}
	for _, c := range []struct {
		name          string
		offset, count uint32
	}{
		{"a length past the end", 0, 1 << 30},
		{"an offset past the end", 1 << 30, 4},
		{"an offset and length that overflow together", 0xFFFFFFF0, 0x20},
	} {
		t.Run(c.name, func(t *testing.T) {
			r0 := h.bytes()
			binary.BigEndian.PutUint32(r0[offFullNameOffset:], c.offset)
			binary.BigEndian.PutUint32(r0[offFullNameLength:], c.count)
			got, err := readHeader(r0)
			if err != nil {
				t.Fatal(err)
			}
			if got.fullName != "" {
				t.Errorf("the title came back as %q", got.fullName)
			}
		})
	}
}

// ⛔ A BAD EXTH LENGTH ENDS THE WALK and keeps what was read. The records are a
// chain: one wrong length makes every offset after it a guess, and metadata is
// a bonus while the book is the point.
func TestAnEXTHBlockThatGoesWrongPartWay(t *testing.T) {
	h := header0{
		compression: compressionNone, textRecords: 1, recordSize: 4096,
		exth: []exthRecord{{100, "Kept"}, {101, "Also kept"}, {103, "Lost"}},
	}
	r0 := h.bytes()
	// Find the third record and give it a length of nothing.
	i := strings.Index(string(r0), "Also kept") + len("Also kept")
	binary.BigEndian.PutUint32(r0[i+4:], 0)
	got, err := readHeader(r0)
	if err != nil {
		t.Fatal(err)
	}
	if got.first(100) != "Kept" || got.first(101) != "Also kept" {
		t.Errorf("the records before the bad one were lost: %v", got.exth)
	}
	if got.first(103) != "" {
		t.Errorf("something was read past the bad length: %q", got.first(103))
	}
}

func TestAnEXTHHeaderThatIsNotThere(t *testing.T) {
	// The flag says there is one and the bytes where it should be say
	// otherwise, which is a producer's bug and not a reason to refuse a book.
	h := header0{compression: compressionNone, textRecords: 1, recordSize: 4096}
	r0 := h.bytes()
	binary.BigEndian.PutUint32(r0[offEXTHFlags:], 0x40)
	got, err := readHeader(r0)
	if err != nil {
		t.Fatal(err)
	}
	if got.exth != nil {
		t.Errorf("EXTH came back as %v from a record that has none", got.exth)
	}

	// And one whose own record count runs past the end of record 0.
	h2 := header0{compression: compressionNone, textRecords: 1, recordSize: 4096, exth: []exthRecord{{100, "An Author"}}}
	r2 := h2.bytes()
	i := strings.Index(string(r2), "EXTH")
	binary.BigEndian.PutUint32(r2[i+8:], 50)
	got2, err := readHeader(r2)
	if err != nil {
		t.Fatal(err)
	}
	if got2.first(100) != "An Author" {
		t.Errorf("the one real record was lost: %v", got2.exth)
	}
}

// ⛔ A HEADER THAT STOPS BETWEEN THE TITLE'S OFFSET AND ITS LENGTH. The two
// fields are four bytes apart, so a header length of 72 covers the first and
// not the second — and a title is only a title when BOTH are there. The
// coverage profile is what asked for this case: nothing else reaches the
// second of the two refusals.
func TestAHeaderThatStopsBetweenTheTitlesTwoFields(t *testing.T) {
	h := header0{
		compression:  compressionNone,
		textRecords:  1,
		recordSize:   4096,
		mobiType:     mobiTypeBook,
		encoding:     encodingUTF8,
		title:        "Not Reachable",
		headerLength: 72,
	}
	got, err := readHeader(h.bytes())
	if err != nil {
		t.Fatal(err)
	}
	if got.fullName != "" {
		t.Errorf("the title came back as %q from a header that holds half of its field pair", got.fullName)
	}
}
