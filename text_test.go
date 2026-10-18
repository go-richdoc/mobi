// Copyright (c) 2026, the go-richdoc/mobi authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package mobi

import (
	"strings"
	"testing"
)

// ⛔ THE TRAILING ENTRIES ARE THE TRAP OF THIS FORMAT. They are appended to a
// record AFTER it is compressed, so they are not text and not compressed
// either, and feeding them to the decompressor yields plausible rubbish at the
// end of every record — not an error, just wrong prose. The book this reader
// was measured against sets flags 0x3: indexing data AND a multibyte overlap,
// on all 259 of its records.
func TestTheBytesAtTheEndOfARecordThatAreNotText(t *testing.T) {
	for _, c := range []struct {
		name  string
		rec   []byte
		flags uint16
		want  string
	}{
		{"no flags, nothing to strip", []byte("text"), 0, "text"},
		{
			// Bit 1: one entry, whose size is written backwards at its end
			// and counts itself. 0x83 has the high bit set, so it is the
			// whole size: three bytes, which is the two of data and itself.
			"an indexing entry", []byte("text\x01\x02\x83"), 0x2, "text",
		},
		{
			// Bit 0 is different: the count is two bits of the last byte, and
			// the entry is that many bytes plus one.
			"a multibyte overlap of one byte", []byte("text\x00"), 0x1, "text",
		},
		{"a multibyte overlap of four bytes", []byte("textabc\x03"), 0x1, "text"},
		{
			// Both, which is what a real book has — and the ORDER is what the
			// first version of this fixture got wrong. The entries are
			// stripped from the END, highest flag first, so the indexing
			// entry's size byte is the record's last byte and the overlap
			// entry sits in front of it.
			"both kinds at once", []byte("text\x00\x81"), 0x3, "text",
		},
		{
			// A size needing two bytes: 0x81 0x00 is (1<<7)|0 = 128 with the
			// high bit on the second byte read. 128 bytes of entry including
			// the two it is written in.
			"a two-byte size", append(append([]byte("text"), make([]byte, 126)...), 0x81, 0x00), 0x2, "text",
		},
		{
			// A bit nothing documents is still a bit, and the walk has to
			// skip the entry rather than ignore the flag: here bit 3.
			"an entry under a flag nobody has named", []byte("textx\x82"), 0x8, "text",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := stripTrailing(c.rec, c.flags)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestTrailingEntriesThatCannotBeThere(t *testing.T) {
	for _, c := range []struct {
		name  string
		rec   []byte
		flags uint16
		says  string
	}{
		{"an entry bigger than the record", []byte("ab\x84"), 0x2, "claim 4 bytes of a 3-byte record"},
		{"an entry at the end of an empty record", []byte{}, 0x2, "empty record"},
		{"a size of zero", []byte("ab\x80"), 0x2, "size is zero"},
		// An indexing entry that eats the whole record exactly, leaving the
		// overlap entry nothing to read its count from.
		{"an overlap in a record with nothing left", []byte{0x81}, 0x3, "no bytes left"},
		{"an overlap claiming more than the record", []byte("\x03"), 0x1, "claim 4 bytes of a 1-byte record"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := stripTrailing(c.rec, c.flags)
			if err == nil {
				t.Fatal("it was accepted")
			}
			if !strings.Contains(err.Error(), c.says) {
				t.Errorf("the refusal says %q, which does not mention %q", err, c.says)
			}
		})
	}
}

// ⛔ A RECORD OF ZEROS would walk the size backwards to the start of the
// record and shift an int by seven a thousand times, which is not an error
// anywhere: it quietly gives zero. The bound is four bytes, and it is the
// reason this test exists.
func TestASizeThatWouldWalkTheWholeRecord(t *testing.T) {
	_, err := stripTrailing(make([]byte, 4096), 0x2)
	if err == nil {
		t.Fatal("4096 zero bytes were accepted as a trailing entry")
	}
	if !strings.Contains(err.Error(), "size is zero") {
		t.Errorf("the refusal says %q", err)
	}
}

func TestAssemblingTheBooksText(t *testing.T) {
	// Two records, uncompressed, and a text length that stops inside the
	// second: the padding after it is the producer's leftovers.
	b := newBook()
	h := header0{
		compression: compressionNone,
		textLength:  uint32(len("<p>one</p><p>two</p>")),
		textRecords: 2,
		recordSize:  16,
		encoding:    encodingUTF8,
		mobiType:    mobiTypeBook,
	}
	b.records = [][]byte{h.bytes(), []byte("<p>one</p><p>two"), []byte("</p>PADDINGPADDING")}
	c, err := openContainer(b.bytes(), defaultMaxRecords)
	if err != nil {
		t.Fatal(err)
	}
	hd, err := readHeader(c.record(0))
	if err != nil {
		t.Fatal(err)
	}
	got, err := bookText(c, hd, defaultMaxTextBytes)
	if err != nil {
		t.Fatal(err)
	}
	// ⛔ A TAG SPLIT ACROSS TWO RECORDS is the reason nothing may be parsed
	// until every record is in hand: "<p>two" ends one record and "</p>"
	// begins the next.
	if string(got) != "<p>one</p><p>two</p>" {
		t.Errorf("got %q", got)
	}
}

func TestTheWaysAssemblingTheTextFails(t *testing.T) {
	build := func(h header0, records ...[]byte) (*container, *header) {
		t.Helper()
		b := newBook()
		b.records = append([][]byte{h.bytes()}, records...)
		c, err := openContainer(b.bytes(), defaultMaxRecords)
		if err != nil {
			t.Fatal(err)
		}
		hd, err := readHeader(c.record(0))
		if err != nil {
			t.Fatal(err)
		}
		return c, hd
	}
	base := header0{compression: compressionNone, textRecords: 2, recordSize: 4096, encoding: encodingUTF8, mobiType: mobiTypeBook, textLength: 8}

	t.Run("a record the file does not hold", func(t *testing.T) {
		c, h := build(base, []byte("12345678"))
		_, err := bookText(c, h, defaultMaxTextBytes)
		if err == nil || !strings.Contains(err.Error(), "the file holds 1") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("less text than the book claims", func(t *testing.T) {
		h0 := base
		h0.textLength = 1 << 20
		c, h := build(h0, []byte("12345678"), []byte("12345678"))
		_, err := bookText(c, h, defaultMaxTextBytes)
		if err == nil || !strings.Contains(err.Error(), "its records hold 16") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("uncompressed text past the limit", func(t *testing.T) {
		c, h := build(base, []byte("12345678"), []byte("12345678"))
		_, err := bookText(c, h, 10)
		if err == nil || !strings.Contains(err.Error(), "passed the limit of 10") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("the limit reached exactly, with records to go", func(t *testing.T) {
		c, h := build(base, []byte("12345678"), []byte("12345678"))
		_, err := bookText(c, h, 8)
		if err == nil || !strings.Contains(err.Error(), "at record 2") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("compressed text past the limit", func(t *testing.T) {
		h0 := base
		h0.compression = compressionPalmDoc
		c, h := build(h0, []byte("12345678"), []byte("12345678"))
		_, err := bookText(c, h, 10)
		if err == nil || !strings.Contains(err.Error(), "past the limit") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("a record that does not decompress", func(t *testing.T) {
		h0 := base
		h0.compression = compressionPalmDoc
		c, h := build(h0, []byte{0x08, 'a'}, []byte("x"))
		_, err := bookText(c, h, defaultMaxTextBytes)
		if err == nil || !strings.Contains(err.Error(), "text record 1") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("a record whose trailing entries are impossible", func(t *testing.T) {
		h0 := base
		h0.extraFlags = 0x2
		c, h := build(h0, []byte{0x84}, []byte("x"))
		_, err := bookText(c, h, defaultMaxTextBytes)
		if err == nil || !strings.Contains(err.Error(), "text record 1") {
			t.Fatalf("err = %v", err)
		}
	})
}

// ⛔ CP1252 IS NOT LATIN-1, and the difference is the punctuation of every
// English book written on Windows: taken as Latin-1, 0x92 — the apostrophe in
// "don't" — becomes U+0092, an invisible control character, and the word comes
// out with a hole in it.
func TestTheEncodingsAMobiComesIn(t *testing.T) {
	for _, c := range []struct {
		name     string
		in       []byte
		encoding uint32
		want     string
	}{
		{"UTF-8 as it stands", []byte("déjà vu — ©"), encodingUTF8, "déjà vu — ©"},
		{"UTF-8 that is not valid is still kept", []byte("a\xffb"), encodingUTF8, "a\xffb"},
		{"CP1252 ASCII", []byte("plain"), encodingCP1252, "plain"},
		{"CP1252 punctuation", []byte("don\x92t \x93say\x94 \x97 \x85"), encodingCP1252, "don’t “say” — …"},
		{"CP1252 above the gap is Latin-1", []byte("caf\xe9 \xa9 \xff"), encodingCP1252, "café © ÿ"},
		{"the five unassigned bytes", []byte("\x81\x8d\x8f\x90\x9d"), encodingCP1252, "�����"},
		{"the euro and the rest of the gap", []byte("\x80\x82\x83\x84\x86\x87\x88\x89\x8a\x8b\x8c\x8e"), encodingCP1252, "€‚ƒ„†‡ˆ‰Š‹Œž"[:len("€‚ƒ„†‡ˆ‰Š‹Œ")] + "Ž"},
		{"the second half of the gap", []byte("\x91\x95\x96\x98\x99\x9a\x9b\x9c\x9e\x9f"), encodingCP1252, "‘•–˜™š›œžŸ"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := decodeText(c.in, c.encoding)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}

	if _, err := decodeText([]byte("x"), 437); err == nil {
		t.Error("an encoding no MOBI uses was accepted")
	}
}
