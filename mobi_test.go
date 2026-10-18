// Copyright (c) 2026, the go-richdoc/mobi authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package mobi

import (
	"errors"
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
)

// plainText is every word the document holds, in order.
func plainText(blocks []richdoc.Block) string {
	var sb strings.Builder
	walkInlines(blocks, func(in richdoc.Inline) {
		if t, ok := in.(richdoc.Text); ok {
			sb.WriteString(t.Value)
		}
	})
	return sb.String()
}

// imageURLs is every picture's URL, in order.
func imageURLs(blocks []richdoc.Block) []string {
	var out []string
	walkInlines(blocks, func(in richdoc.Inline) {
		if img, ok := in.(richdoc.Image); ok {
			out = append(out, img.URL)
		}
	})
	return out
}

func walkInlines(blocks []richdoc.Block, f func(richdoc.Inline)) {
	var inlines func([]richdoc.Inline)
	inlines = func(in []richdoc.Inline) {
		for _, i := range in {
			f(i)
			switch v := i.(type) {
			case richdoc.Emph:
				inlines(v.Inlines)
			case richdoc.Strong:
				inlines(v.Inlines)
			case richdoc.Link:
				inlines(v.Inlines)
			}
		}
	}
	var walk func([]richdoc.Block)
	walk = func(bs []richdoc.Block) {
		for _, b := range bs {
			switch v := b.(type) {
			case richdoc.Paragraph:
				inlines(v.Inlines)
			case richdoc.Heading:
				inlines(v.Inlines)
			case richdoc.BlockQuote:
				walk(v.Blocks)
			case richdoc.List:
				for _, it := range v.Items {
					walk(it.Blocks)
				}
			}
		}
	}
	walk(blocks)
}

func TestABookReadFromEndToEnd(t *testing.T) {
	text := `<html><head><guide><reference title="Start" type="text" filepos=0000000100/></guide></head>` +
		`<body><h1>A Chapter</h1><p>The <i>first</i> paragraph.</p>` +
		`<p><a filepos="0000000200">a link inside the book</a></p>` +
		`<mbp:pagebreak/><p>After the page break.</p></body></html>`
	doc, err := Parse(plainBook(t, text, header0{title: "The Title"}), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Blocks) < 3 {
		t.Fatalf("%d blocks: %#v", len(doc.Blocks), doc.Blocks)
	}
	if h, ok := doc.Blocks[0].(richdoc.Heading); !ok || h.Level != 1 {
		t.Errorf("the first block is %#v, want a level-one heading", doc.Blocks[0])
	}
	got := plainText(doc.Blocks)
	for _, want := range []string{"A Chapter", "first", "paragraph", "After the page break"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q is missing from %q", want, got)
		}
	}
	// ⛔ A filepos LINK COMES OUT AS PLAIN TEXT, deliberately: MOBI's internal
	// links are byte offsets into the decompressed text, not names, and a link
	// that looks clickable and goes nowhere is worse than none. The WORDS are
	// kept in every case.
	if !strings.Contains(got, "a link inside the book") {
		t.Errorf("the text of an internal link was lost: %q", got)
	}
	for _, in := range doc.Blocks {
		if p, ok := in.(richdoc.Paragraph); ok {
			for _, i := range p.Inlines {
				if l, ok := i.(richdoc.Link); ok {
					t.Errorf("a link survived with URL %q, and a filepos is not a URL", l.URL)
				}
			}
		}
	}
}

func TestWhatTheBookSaysAboutItself(t *testing.T) {
	t.Run("the EXTH records", func(t *testing.T) {
		h := header0{
			title: "The Title In The Header",
			exth: []exthRecord{
				{503, "The Title As Updated"},
				{100, "First Author"},
				{100, "Second Author"},
				{101, "A Publisher"},
				{103, "What it is about."},
				{104, "978-0000000000"},
				{105, "Fiction"},
				{105, "Travel"},
				{106, "1998-06-02T00:00:00+00:00"},
				{109, "Public domain"},
				{113, "B000000000"},
				{524, "en"},
				{108, "calibre (9.5.0)"},
			},
		}
		doc, err := Parse(plainBook(t, "<p>text</p>", h), Limits{})
		if err != nil {
			t.Fatal(err)
		}
		// ⛔ 503 WINS over the header's own title. The header's name field is
		// fixed-length and written first; a producer that retitles a book
		// afterwards writes 503 and leaves the header alone, so the header's
		// title is the OLD one.
		for k, want := range map[string]string{
			"title":       "The Title As Updated",
			"author":      "First Author; Second Author",
			"publisher":   "A Publisher",
			"description": "What it is about.",
			"isbn":        "978-0000000000",
			"subject":     "Fiction; Travel",
			"date":        "1998-06-02T00:00:00+00:00",
			"rights":      "Public domain",
			"asin":        "B000000000",
			"language":    "en",
			"producer":    "calibre (9.5.0)",
		} {
			if got := doc.Meta[k]; got != want {
				t.Errorf("%s is %q, want %q", k, got, want)
			}
		}
	})

	t.Run("the header's title when there is no 503", func(t *testing.T) {
		doc, err := Parse(plainBook(t, "<p>text</p>", header0{title: "Only In The Header"}), Limits{})
		if err != nil {
			t.Fatal(err)
		}
		if doc.Meta["title"] != "Only In The Header" {
			t.Errorf("the title is %q", doc.Meta["title"])
		}
	})

	t.Run("the database name when there is no title at all", func(t *testing.T) {
		// ⛔ The last resort, and a poor title: the Palm database name is the
		// title with its spaces turned into underscores, truncated to 31
		// bytes. Better than nothing, which is what a book with no title
		// would otherwise show in a library.
		doc, err := Parse(plainBook(t, "<p>text</p>", header0{}), Limits{})
		if err != nil {
			t.Fatal(err)
		}
		if doc.Meta["title"] != "A Test Book" {
			t.Errorf("the title is %q, want the database name with its underscores undone", doc.Meta["title"])
		}
	})

	t.Run("a value of nothing but spaces is not a value", func(t *testing.T) {
		doc, err := Parse(plainBook(t, "<p>text</p>", header0{
			title: "A Title", exth: []exthRecord{{101, "   "}},
		}), Limits{})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := doc.Meta["publisher"]; ok {
			t.Errorf("publisher came through as %q", doc.Meta["publisher"])
		}
	})
}

func TestTheWaysParseRefuses(t *testing.T) {
	wrongCreator := newBook()
	wrongCreator.creator = "TEXt"
	wrongCreator.records = [][]byte{header0{compression: compressionNone, textRecords: 1, recordSize: 4096}.bytes(), []byte("x")}

	noText := newBook()
	noText.records = [][]byte{header0{
		compression: compressionNone, textRecords: 2, recordSize: 4096, textLength: 4, encoding: encodingUTF8,
	}.bytes()}

	// ⛔ The encoding is set in the HEADER rather than poked into the bytes.
	// The first version of this searched the file for "MOBI" and found the
	// Palm database's CREATOR field at offset 64 — the identifier in record 0
	// is the second one — so it corrupted the record list instead, and the
	// test then passed on a refusal about something else entirely.
	badEncodingBook := newBook()
	badEncodingBook.records = [][]byte{
		header0{
			compression: compressionNone, textRecords: 1, recordSize: 4096,
			textLength: 8, mobiType: mobiTypeBook, encoding: 437,
		}.bytes(),
		[]byte("<p>x</p>"),
	}

	for _, c := range []struct {
		name   string
		in     []byte
		says   string
		format bool
	}{
		{"something that is not a Palm database", []byte("hello"), "cannot hold", true},
		{"a Palm database that is not a MOBI", wrongCreator.bytes(), `creator is "TEXt"`, true},
		{"a book whose text records are not in the file", noText.bytes(), "the file holds 0", false},
		{"an encoding no MOBI uses", badEncodingBook.bytes(), "text encoding 437", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(c.in, Limits{})
			if err == nil {
				t.Fatal("it was accepted")
			}
			if !strings.Contains(err.Error(), c.says) {
				t.Errorf("the refusal says %q, which does not mention %q", err, c.says)
			}
			if got := errors.Is(err, ErrFormat); got != c.format {
				t.Errorf("errors.Is(err, ErrFormat) is %v, want %v, for %q", got, c.format, err)
			}
		})
	}
}

func TestTheLimitsAreTheDefaultsWhenNobodySaysOtherwise(t *testing.T) {
	got := Limits{}.orDefaults()
	want := Limits{
		MaxRecords:         defaultMaxRecords,
		MaxTextBytes:       defaultMaxTextBytes,
		MaxImageBytes:      defaultMaxImageBytes,
		MaxTotalImageBytes: defaultMaxTotalImageBytes,
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	// And a caller's own numbers are kept, including the two this type passes
	// straight on to the HTML reader.
	mine := Limits{MaxRecords: 1, MaxTextBytes: 2, MaxImageBytes: 3, MaxTotalImageBytes: 4, MaxDepth: 5, MaxNodes: 6}
	if mine.orDefaults() != mine {
		t.Errorf("%+v came back as %+v", mine, mine.orDefaults())
	}
}

// ⛔ THE LIMIT HANDED TO THE HTML READER HAS TO ARRIVE. A Limits that sets
// MaxNodes and a reader that never sees it is a bound that reads as enforced
// and is not, which is the worst kind: nothing fails until a crafted book
// arrives.
func TestTheHtmlReadersLimitsArrive(t *testing.T) {
	text := "<p>" + strings.Repeat("<b>deep", 60) + "</p>"
	if _, err := Parse(plainBook(t, text, header0{}), Limits{MaxDepth: 4}); err == nil {
		t.Fatal("a document far deeper than the limit was accepted")
	}
	if _, err := Parse(plainBook(t, text, header0{}), Limits{MaxNodes: 3}); err == nil {
		t.Fatal("a document with far more nodes than the limit was accepted")
	}
	// The same book, with the limits left alone, is read.
	if _, err := Parse(plainBook(t, text, header0{}), Limits{}); err != nil {
		t.Fatalf("the defaults refused a small book: %v", err)
	}
}

// ⛔ A COMPRESSED BOOK END TO END, because every other test here reads an
// uncompressed one: the bytes below are typed from the format's description,
// and they are what a real book's records look like.
func TestACompressedBookReadFromEndToEnd(t *testing.T) {
	// "<p>Hello Hello</p>" written as PalmDOC: nine literal bytes, then a
	// back-reference that repeats "Hello".
	//
	// ⛔ THE DISTANCE IS COUNTED FROM THE END OF THE OUTPUT SO FAR, and the
	// first version of this fixture had it one short: distance 5 into
	// "<p>Hello " lands on the "e", and the test read "Hello ello" — which is
	// exactly what a reader with an off-by-one would produce on a real book,
	// and exactly why the fixture is typed out rather than generated.
	compressed := []byte{
		'<', 'p', '>', 'H', 'e', 'l', 'l', 'o', ' ',
		0x80, (6 << 3) | (5 - 3), // distance 6, length 5: "Hello"
		'<', '/', 'p', '>',
	}
	h := header0{
		compression: compressionPalmDoc,
		textLength:  uint32(len("<p>Hello Hello</p>")),
		textRecords: 1,
		recordSize:  4096,
		encoding:    encodingUTF8,
		mobiType:    mobiTypeBook,
		title:       "A Compressed Book",
	}
	b := newBook()
	b.records = [][]byte{h.bytes(), compressed}
	doc, err := Parse(b.bytes(), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if got := plainText(doc.Blocks); got != "Hello Hello" {
		t.Errorf("got %q, want %q", got, "Hello Hello")
	}
}

// ⛔ A REFUSAL FROM THE HEADER HAS TO COME OUT OF Parse. The header's own
// tests call readHeader directly; this one is the path a caller takes, and
// the coverage profile is what said nothing walked it.
func TestParsePassesTheHeadersRefusalOn(t *testing.T) {
	b := newBook()
	b.records = [][]byte{
		header0{
			compression: compressionPalmDoc, textRecords: 1, recordSize: 4096,
			mobiType: mobiTypeKF8, title: "An AZW3",
		}.bytes(),
		[]byte("x"),
	}
	_, err := Parse(b.bytes(), Limits{})
	if err == nil || !strings.Contains(err.Error(), "KF8") {
		t.Fatalf("err = %v", err)
	}
}
