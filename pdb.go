// Copyright (c) 2026, the go-richdoc/mobi authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package mobi

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// ErrFormat means the bytes are not a Mobipocket file at all — the wrong
// creator, or too short to hold a Palm database header.
//
// ⛔ It is deliberately NOT returned for a MOBI this package will not read: a
// DRM'd book, a HUFF/CDIC one and a KF8 one are all perfectly good
// Mobipocket files, and a caller sniffing formats by trying readers in turn
// must not be told "not mine" about a file that IS mine and is refused for a
// reason worth repeating to a person.
var ErrFormat = errors.New("mobi: not a Mobipocket file")

// pdbHeaderSize is the fixed part of a Palm database header. The record list
// follows it immediately, eight bytes an entry.
const pdbHeaderSize = 78

// a container is the Palm database: the records, and where each one starts and
// ends.
type container struct {
	name    string
	typ     string
	creator string
	src     []byte
	// bounds has one more entry than there are records: bounds[i] to
	// bounds[i+1] is record i.
	bounds []int64
}

// openContainer reads the header and the record list, and checks that every
// record is inside the file before anything asks for one.
//
// ⛔ THE END OF THE LAST RECORD IS NOT WRITTEN DOWN. A record entry gives a
// start and nothing else, and the format's own documentation does not say how
// the last one ends; every reader takes it to be the end of the file, and so
// does this one. It is worth saying out loud because it is the one bound here
// that is a convention rather than a field.
func openContainer(src []byte, maxRecords int) (*container, error) {
	if len(src) < pdbHeaderSize+8 {
		return nil, fmt.Errorf("%w: %d bytes cannot hold a Palm database header and one record", ErrFormat, len(src))
	}
	count := int(binary.BigEndian.Uint16(src[76:]))
	if count == 0 {
		return nil, fmt.Errorf("%w: a Palm database with no records", ErrFormat)
	}
	if count > maxRecords {
		return nil, fmt.Errorf("mobi: %d records, and the limit is %d", count, maxRecords)
	}
	listEnd := pdbHeaderSize + 8*count
	if len(src) < listEnd {
		return nil, fmt.Errorf("%w: a record list of %d entries needs %d bytes and the file is %d",
			ErrFormat, count, listEnd, len(src))
	}

	c := &container{
		name:    string(trimNul(src[0:32])),
		typ:     string(src[60:64]),
		creator: string(src[64:68]),
		src:     src,
		bounds:  make([]int64, count+1),
	}
	for i := range count {
		c.bounds[i] = int64(binary.BigEndian.Uint32(src[pdbHeaderSize+8*i:]))
	}
	c.bounds[count] = int64(len(src))
	for i := range count {
		// ⛔ Non-decreasing AND inside the file, checked for every record
		// here rather than when one is asked for. Each of these offsets is a
		// number IN THE FILE used to slice a byte slice: one that points
		// backwards, or past the end, is a panic in whatever asks for that
		// record later — a long way from the bytes that caused it.
		if c.bounds[i] < int64(listEnd) || c.bounds[i] > c.bounds[i+1] {
			return nil, fmt.Errorf("%w: record %d runs from %d to %d, which is not inside a %d-byte file",
				ErrFormat, i, c.bounds[i], c.bounds[i+1], len(src))
		}
	}
	return c, nil
}

// count is how many records the database holds.
func (c *container) count() int { return len(c.bounds) - 1 }

// record is record i's bytes, or nil when there is no such record.
//
// ⛔ A MISSING RECORD IS nil AND NOT AN ERROR, because the numbers that reach
// this come from the book: a picture's record index, the first image index,
// the count of text records. A book that names a record it does not have has
// lost that picture, and losing a picture must not lose the book — which is
// the same rule the EPUB reader applies to an image its ZIP does not hold.
func (c *container) record(i int) []byte {
	if i < 0 || i >= c.count() {
		return nil
	}
	return c.src[c.bounds[i]:c.bounds[i+1]]
}

func trimNul(b []byte) []byte {
	if i := strings.IndexByte(string(b), 0); i >= 0 {
		return b[:i]
	}
	return b
}
