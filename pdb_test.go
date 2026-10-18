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

func TestTheRecordsOfAPalmDatabase(t *testing.T) {
	b := newBook()
	b.records = [][]byte{[]byte("zero"), []byte("one"), []byte("the last one")}
	c, err := openContainer(b.bytes(), defaultMaxRecords)
	if err != nil {
		t.Fatal(err)
	}
	if c.count() != 3 {
		t.Fatalf("%d records, want three", c.count())
	}
	if c.name != "A_Test_Book" || c.typ != "BOOK" || c.creator != "MOBI" {
		t.Errorf("name %q type %q creator %q", c.name, c.typ, c.creator)
	}
	for i, want := range []string{"zero", "one", "the last one"} {
		if got := string(c.record(i)); got != want {
			t.Errorf("record %d is %q, want %q", i, got, want)
		}
	}
	// ⛔ The last record's END is the end of the file and nothing says so in
	// the file, which is why it is asserted here rather than assumed: a
	// reader that stopped one record short would lose the last chapter of
	// every book.
	if got := string(c.record(2)); !strings.HasSuffix(got, "last one") {
		t.Errorf("the last record is %q, so its end was taken from somewhere else", got)
	}
	// A record that is not there is nil and not a panic, because the numbers
	// that reach this come out of the book.
	for _, i := range []int{-1, 3, 1 << 20} {
		if c.record(i) != nil {
			t.Errorf("record %d came back with something in it", i)
		}
	}
}

func TestWhatIsNotAPalmDatabase(t *testing.T) {
	short := newBook()
	short.records = [][]byte{[]byte("x")}
	whole := short.bytes()

	noRecords := append([]byte{}, whole...)
	binary.BigEndian.PutUint16(noRecords[76:], 0)

	tooManyClaimed := append([]byte{}, whole...)
	binary.BigEndian.PutUint16(tooManyClaimed[76:], 400)

	backwards := append([]byte{}, whole...)
	binary.BigEndian.PutUint32(backwards[pdbHeaderSize:], 4) // inside the header itself

	pastTheEnd := append([]byte{}, whole...)
	binary.BigEndian.PutUint32(pastTheEnd[pdbHeaderSize:], uint32(len(whole)+1000))

	for _, c := range []struct {
		name   string
		in     []byte
		says   string
		format bool // is it ErrFormat, "not one of ours at all"?
	}{
		{"nothing", nil, "cannot hold", true},
		{"a header and no record list", make([]byte, 80), "cannot hold", true},
		{"a database claiming no records", noRecords, "no records", true},
		{"a record list longer than the file", tooManyClaimed, "record list of 400", true},
		{"a record starting inside the header", backwards, "not inside", true},
		{"a record starting past the end", pastTheEnd, "not inside", true},
		{"more records than the limit allows", whole, "and the limit is", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			limit := defaultMaxRecords
			if !c.format {
				limit = 0 // the one case that is a limit and not a format
			}
			_, err := openContainer(c.in, limit)
			if err == nil {
				t.Fatal("it was accepted")
			}
			if !strings.Contains(err.Error(), c.says) {
				t.Errorf("the refusal says %q, which does not mention %q", err, c.says)
			}
			// ⛔ ErrFormat is checked BOTH ways round. A limit reported as
			// "not a MOBI" would make a caller that sniffs formats move on to
			// the next reader, and the next reader would say the same.
			if got := errors.Is(err, ErrFormat); got != c.format {
				t.Errorf("errors.Is(err, ErrFormat) is %v, want %v, for %q", got, c.format, err)
			}
		})
	}
}

func TestANameWithNoTerminator(t *testing.T) {
	// A database name fills its 32 bytes with no room for the terminator the
	// format asks for, which is a real file and not a malformed one.
	b := newBook()
	b.name = strings.Repeat("n", 32)
	b.records = [][]byte{[]byte("x")}
	c, err := openContainer(b.bytes(), defaultMaxRecords)
	if err != nil {
		t.Fatal(err)
	}
	if c.name != strings.Repeat("n", 32) {
		t.Errorf("the name came back as %q", c.name)
	}
}
