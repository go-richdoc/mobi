// Copyright (c) 2026, the go-richdoc/mobi authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package mobi

import (
	"strings"
	"testing"
)

// The cases in this file were asked for by a mutation sweep: each one is a
// branch the suite exercised without being able to tell whether it was there.
//
// ⛔ A SURVIVOR IS NOT YET A DEFECT, and two of this sweep's five were not: the
// title's two "is the field there" guards gave the same answer as the code
// after them, and they are gone rather than tested. The three below are
// different — each one changes what a reader hands back.

// ⛔ AN UNRESOLVABLE PICTURE LEAVES NO TAG BEHIND, asserted on the BYTES.
// Nothing downstream can see the difference — the HTML reader drops an img
// with an empty src too, which was measured rather than assumed — so a test
// written on the document could not hold this, and a mutation sweep survived
// here until the assertion moved to the layer that owns the output.
func TestAnUnresolvablePictureLeavesNoTag(t *testing.T) {
	got := string(pictures(
		[]byte(`<p>x<img recindex="00001" alt="gone"/>y</p>`),
		func(int) (string, bool) { return "", false },
	))
	if strings.Contains(got, "<img") {
		t.Errorf("an img survived with nothing to show: %q", got)
	}
	if want := `<p>xy</p>`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ⛔ A TAG CARRYING BOTH a src and a recindex means the src. That is what every
// other reader of the book's HTML will show, and a reader that preferred the
// record would put a different picture on the page from the one the author
// sees. Nothing distinguished the two before this.
func TestAPictureNamedBothWaysAtOnce(t *testing.T) {
	src := bookWithPictures(t,
		`<p><img src="https://example.com/real.png" recindex="00001"/></p>`,
		jpegBytes)
	doc, err := Parse(src, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	urls := imageURLs(doc.Blocks)
	if len(urls) != 1 {
		t.Fatalf("%d pictures, want one: %.40q", len(urls), urls)
	}
	if urls[0] != "https://example.com/real.png" {
		t.Errorf("the picture came out as %.60q; the src the author wrote has to win", urls[0])
	}
}

// ⛔ A REFERENCE OF ZERO IS NOT A REFERENCE, and the record before the first
// image is the trap: pictures are counted from one, so "zero" would resolve to
// firstImage-1 — a text record, an index, or, as here, a picture that belongs
// to nothing. Reading it would put a picture on the page that the book never
// referred to.
//
// The fixture is built so that the record before the first image IS a valid
// JPEG, because that is the only arrangement in which the guard and its
// absence differ.
func TestAReferenceOfZeroDoesNotReachTheRecordBefore(t *testing.T) {
	h := header0{
		compression: compressionNone,
		textLength:  uint32(len(`<p>before<img alt="names nothing"/>after</p>`)),
		textRecords: 1,
		recordSize:  4096,
		encoding:    encodingUTF8,
		mobiType:    mobiTypeBook,
		firstImage:  3, // record 1 is the text, record 2 a picture nothing names
		title:       "A Book With A Picture Before Its First",
	}
	b := newBook()
	b.records = [][]byte{
		h.bytes(),
		[]byte(`<p>before<img alt="names nothing"/>after</p>`),
		jpegBytes, // record 2: a real picture, and firstImage says 3
		pngBytes,  // record 3: the first picture proper
	}
	doc, err := Parse(b.bytes(), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if urls := imageURLs(doc.Blocks); len(urls) != 0 {
		t.Errorf("a picture came from the record BEFORE the first image: %.40q", urls)
	}
	if got := plainText(doc.Blocks); !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Errorf("the text around the dropped picture is %q", got)
	}
}

// ⛔ THE LIMIT REACHED EXACTLY, with a COMPRESSED record still to go. The
// uncompressed path says so itself, and the decompressor would too — but with
// a different message, naming a per-record limit of zero bytes rather than the
// budget the caller set. One message for both paths is the point of the check,
// and this is what holds it.
func TestTheTextLimitReachedExactlyOnACompressedBook(t *testing.T) {
	h := header0{
		compression: compressionPalmDoc,
		textLength:  16,
		textRecords: 2,
		recordSize:  4096,
		encoding:    encodingUTF8,
		mobiType:    mobiTypeBook,
	}
	b := newBook()
	b.records = [][]byte{h.bytes(), []byte("12345678"), []byte("12345678")}
	c, err := openContainer(b.bytes(), defaultMaxRecords)
	if err != nil {
		t.Fatal(err)
	}
	hd, err := readHeader(c.record(0))
	if err != nil {
		t.Fatal(err)
	}
	_, err = bookText(c, hd, 8)
	if err == nil {
		t.Fatal("a book twice the limit was read")
	}
	if !strings.Contains(err.Error(), "the text passed the limit of 8 bytes at record 2") {
		t.Errorf("the refusal says %q, and it should name the caller's own budget", err)
	}
}
