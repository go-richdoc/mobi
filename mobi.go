// Copyright (c) 2026, the go-richdoc/mobi authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package mobi

import (
	"fmt"
	"strings"

	"github.com/go-richdoc/html"
	"github.com/go-richdoc/richdoc"
)

// Limits bound what a file can make this reader do.
//
// ⛔ Every one of these bounds an allocation decided by a number IN THE FILE.
// A MOBI's record 0 says how long its text is in a four-byte field: it costs
// the author of that file nothing to claim four gigabytes, and a reader that
// believes it allocates them.
type Limits struct {
	// MaxRecords is how many records the Palm database may hold. The format
	// writes the count in two bytes, so this is a bound on WORK rather than on
	// one allocation: sixty-five thousand records of four kilobytes is a
	// quarter of a gigabyte of decompression.
	MaxRecords int

	// MaxTextBytes is how much text this will decompress in all, across every
	// record. One record under the ceiling says nothing about two hundred of
	// them.
	MaxTextBytes int64

	// MaxImageBytes is how large one picture may be and MaxTotalImageBytes how
	// much picture this will carry in all. They are separate from the text
	// because a book is mostly pictures and the two fail differently: one
	// enormous cover, or ten thousand small decorations.
	MaxImageBytes      int64
	MaxTotalImageBytes int64

	// MaxDepth and MaxNodes are handed to the HTML reader. They are named here
	// so that a caller sets one Limits rather than two.
	MaxDepth int
	MaxNodes int
}

const (
	defaultMaxRecords         = 65535
	defaultMaxTextBytes       = 64 << 20
	defaultMaxImageBytes      = 32 << 20
	defaultMaxTotalImageBytes = 256 << 20
)

func (l Limits) orDefaults() Limits {
	if l.MaxRecords <= 0 {
		l.MaxRecords = defaultMaxRecords
	}
	if l.MaxTextBytes <= 0 {
		l.MaxTextBytes = defaultMaxTextBytes
	}
	if l.MaxImageBytes <= 0 {
		l.MaxImageBytes = defaultMaxImageBytes
	}
	if l.MaxTotalImageBytes <= 0 {
		l.MaxTotalImageBytes = defaultMaxTotalImageBytes
	}
	return l
}

// Parse reads a MOBI book into a document.
//
// The whole of the format is four steps: the Palm database gives the records,
// record 0 says how the text is compressed and where the pictures start, the
// text records concatenate into one HTML document, and that document goes
// through the HTML reader.
func Parse(src []byte, lim Limits) (*richdoc.Document, error) {
	lim = lim.orDefaults()
	c, err := openContainer(src, lim.MaxRecords)
	if err != nil {
		return nil, err
	}
	// ⛔ The creator is checked and the TYPE is not. Every MOBI says creator
	// MOBI; the type is BOOK for a book and PDOC for a personal document, and
	// both are read the same way. A reader that insisted on BOOK would refuse
	// everything a person sent to their own Kindle.
	if c.creator != "MOBI" {
		return nil, fmt.Errorf("%w: the Palm database's creator is %q, and a MOBI's is MOBI", ErrFormat, c.creator)
	}
	h, err := readHeader(c.record(0))
	if err != nil {
		return nil, err
	}
	raw, err := bookText(c, h, lim.MaxTextBytes)
	if err != nil {
		return nil, err
	}
	text, err := decodeText(raw, h.encoding)
	if err != nil {
		return nil, err
	}
	withPictures := pictures([]byte(text), imageSet(c, h, lim))
	doc, err := html.Parse(withPictures, htmlLimits(lim))
	if err != nil {
		return nil, fmt.Errorf("mobi: %q: %w", h.fullName, err)
	}
	doc.Meta = h.meta(c)
	return doc, nil
}

// meta is what the book says about itself: the MOBI header's own title, and
// the EXTH records a producer fills in.
//
// ⛔ The keys are the EPUB reader's keys, not MOBI's numbers. A caller
// converting a book to PDF asks for "title" and "author", and a map keyed by
// 503 and 100 would make every one of them learn this format.
func (h *header) meta(c *container) map[string]string {
	m := map[string]string{}
	set := func(k, v string) {
		if v = strings.TrimSpace(v); v != "" {
			m[k] = v
		}
	}
	// ⛔ EXTH 503 ("updated title") WINS over the MOBI header's own name. The
	// header's name field is fixed-length and written first; a producer that
	// changes the title afterwards writes 503 and leaves the header alone, so
	// the header's title is the OLD one. The fallback is the database name,
	// which is the title with its spaces turned into underscores — a poor
	// title, but better than none.
	title := h.first(503)
	if title == "" {
		title = h.fullName
	}
	if title == "" {
		title = strings.ReplaceAll(c.name, "_", " ")
	}
	set("title", title)
	set("author", strings.Join(h.exth[100], "; "))
	set("publisher", h.first(101))
	set("description", h.first(103))
	set("isbn", h.first(104))
	set("subject", strings.Join(h.exth[105], "; "))
	set("date", h.first(106))
	set("rights", h.first(109))
	set("asin", h.first(113))
	set("language", h.first(524))
	set("producer", h.first(108))
	return m
}
