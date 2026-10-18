// Copyright (c) 2026, the go-richdoc/mobi authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package mobi

import (
	"bytes"
	"strings"
	"testing"
)

// a one-pixel JPEG, PNG and GIF, by their magic numbers: what is being tested
// is the sniffing and the record arithmetic, not a decoder.
var (
	jpegBytes = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, make([]byte, 20)...)
	pngBytes  = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 20)...)
	gifBytes  = append([]byte("GIF89a"), make([]byte, 20)...)
	bmpBytes  = append([]byte("BM"), make([]byte, 20)...)
)

// ⛔ BASE 32, AND ONE-BASED, and both were measured rather than assumed: of
// the 164 kindle:embed references in Project Gutenberg's own MOBI of Pride and
// Prejudice, all 164 land on a JPEG read one-based and 163 do read zero-based.
// Read as hexadecimal the numbers would be wrong for every picture past the
// thirty-first and would still look right for the first nine.
func TestTheTwoWaysABookNamesAPicture(t *testing.T) {
	for _, c := range []struct {
		in   string
		want int
		ok   bool
	}{
		{"kindle:embed:0001", 1, true},
		{"kindle:embed:0009", 9, true},
		{"kindle:embed:000A", 10, true},
		{"kindle:embed:000V", 31, true},
		{"kindle:embed:0010", 32, true}, // sixteen if anybody reads it as hex
		{"kindle:embed:003C", 108, true},
		{"kindle:embed:003C?mime=image/jpeg", 108, true},
		{"kindle:embed:0001#anchor", 1, true},
		{"kindle:embed:", 0, false},
		{"kindle:embed:00ZZ", 0, false}, // W to Z are not base-32 digits
		{"kindle:embed:0001 ", 0, false},
		{"kindle:flow:0004?mime=image/svg+xml", 0, false},
		{"images/cover.jpg", 0, false},
		{"", 0, false},
		{"kindle:embed:" + strings.Repeat("V", 20), 0, false}, // too long to mean anything
	} {
		t.Run(c.in, func(t *testing.T) {
			got, ok := kindleEmbed(c.in)
			if ok != c.ok || got != c.want {
				t.Errorf("got %d, %v; want %d, %v", got, ok, c.want, c.ok)
			}
		})
	}
}

func TestSniffingAPictureRecord(t *testing.T) {
	for _, c := range []struct {
		name string
		in   []byte
		want string
	}{
		{"a JPEG", jpegBytes, "image/jpeg"},
		{"a PNG", pngBytes, "image/png"},
		{"a GIF 89a", gifBytes, "image/gif"},
		{"a GIF 87a", append([]byte("GIF87a"), 0, 0), "image/gif"},
		{"a BMP", bmpBytes, "image/bmp"},
		{"an INDX record", []byte("INDX\x00\x00\x00\xc0"), ""},
		{"a FLIS record", []byte("FLIS\x00\x00\x00\x08"), ""},
		{"nothing", nil, ""},
		{"one byte", []byte{0xFF}, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := sniff(c.in); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// bookWithPictures builds a book whose text names pictures both ways.
func bookWithPictures(t *testing.T, text string, pics ...[]byte) []byte {
	t.Helper()
	h := header0{
		compression: compressionNone,
		textLength:  uint32(len(text)),
		textRecords: 1,
		recordSize:  4096,
		encoding:    encodingUTF8,
		mobiType:    mobiTypeBook,
		firstImage:  2, // record 0 is the header, record 1 the text
		title:       "A Book With Pictures",
	}
	b := newBook()
	b.records = append([][]byte{h.bytes(), []byte(text)}, pics...)
	return b.bytes()
}

func TestAPictureNamedEitherWay(t *testing.T) {
	src := bookWithPictures(t,
		`<p>one <img recindex="00001" alt="first"/> two <img src="kindle:embed:0002?mime=image/png"/></p>`,
		jpegBytes, pngBytes)
	doc, err := Parse(src, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	urls := imageURLs(doc.Blocks)
	if len(urls) != 2 {
		t.Fatalf("%d pictures, want two: %q", len(urls), urls)
	}
	if !strings.HasPrefix(urls[0], "data:image/jpeg;base64,") {
		t.Errorf("the recindex picture came out as %.40q", urls[0])
	}
	if !strings.HasPrefix(urls[1], "data:image/png;base64,") {
		t.Errorf("the kindle:embed picture came out as %.40q", urls[1])
	}
}

func TestThePicturesABookLosesAndKeeps(t *testing.T) {
	for _, c := range []struct {
		name   string
		text   string
		pics   [][]byte
		lim    Limits
		want   []string
		anchor string
	}{
		{
			name: "a record the file does not hold",
			text: `<p>a<img recindex="00009"/>b</p>`,
			pics: [][]byte{jpegBytes},
			want: nil,
		},
		{
			name: "a record that is not a picture at all",
			text: `<p>a<img recindex="00001"/>b</p>`,
			pics: [][]byte{[]byte("INDX\x00\x00\x00\xc0")},
			want: nil,
		},
		{
			name: "a picture bigger than the limit",
			text: `<p><img recindex="00001"/></p>`,
			pics: [][]byte{jpegBytes},
			lim:  Limits{MaxImageBytes: 4},
			want: nil,
		},
		{
			name: "more pictures in all than the limit",
			text: `<p><img recindex="00001"/><img recindex="00002"/></p>`,
			pics: [][]byte{jpegBytes, pngBytes},
			lim:  Limits{MaxTotalImageBytes: 30},
			want: []string{"data:image/jpeg"},
		},
		{
			name: "a reference with no number in it",
			text: `<p><img recindex="not a number"/></p>`,
			pics: [][]byte{jpegBytes},
			want: nil,
		},
		{
			name: "a reference of zero, which is no picture",
			text: `<p><img recindex="00000"/></p>`,
			pics: [][]byte{jpegBytes},
			want: nil,
		},
		{
			// ⛔ KEPT, not dropped: a kindle:flow part lives in the KF8 half
			// of a combined file, which this reader does not read. The URL is
			// still a name a person can look up, which is the EPUB reader's
			// rule for an image it cannot resolve.
			name: "a flow reference, which is not an image record",
			text: `<p><img src="kindle:flow:0004?mime=image/svg+xml"/></p>`,
			pics: [][]byte{jpegBytes},
			want: []string{"kindle:flow:0004?mime=image/svg+xml"},
		},
		{
			name: "an ordinary URL, left alone",
			text: `<p><img src="https://example.com/a.png"/></p>`,
			pics: [][]byte{jpegBytes},
			want: []string{"https://example.com/a.png"},
		},
		{
			name: "the same picture twice, read once",
			text: `<p><img recindex="00001"/><img recindex="00001"/></p>`,
			pics: [][]byte{jpegBytes},
			want: []string{"data:image/jpeg", "data:image/jpeg"},
		},
		{
			name: "the same missing picture twice",
			text: `<p><img recindex="00007"/><img recindex="00007"/></p>`,
			pics: [][]byte{jpegBytes},
			want: nil,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc, err := Parse(bookWithPictures(t, c.text, c.pics...), c.lim)
			if err != nil {
				t.Fatal(err)
			}
			urls := imageURLs(doc.Blocks)
			if len(urls) != len(c.want) {
				t.Fatalf("%d pictures, want %d: %.60q", len(urls), len(c.want), urls)
			}
			for i, w := range c.want {
				if !strings.HasPrefix(urls[i], w) {
					t.Errorf("picture %d is %.40q, want it to start %q", i, urls[i], w)
				}
			}
			// ⛔ And the text around it survives in every one of these cases.
			// A picture is a decoration; losing one must never lose a word.
			if c.name == "a record the file does not hold" && !strings.Contains(plainText(doc.Blocks), "b") {
				t.Error("the text after a missing picture was lost with it")
			}
		})
	}
}

// ⛔ EVERY TOKEN THAT IS NOT AN img TAG COMES THROUGH AS IT WAS READ, which is
// what makes this rewrite safe on machine-written HTML: comments, doctypes,
// entities, unquoted attributes, a tag that never closes, and the word
// "recindex" in prose.
func TestTheRewriteLeavesEverythingElseAlone(t *testing.T) {
	for _, in := range []string{
		`<!DOCTYPE html><html><body><p>plain</p></body></html>`,
		`<p>text about recindex in prose</p>`,
		`<!-- a comment --><p>after</p>`,
		`<p class=unquoted>value</p>`,
		`<p>an &amp; entity</p>`,
		`<p>never closed`,
		`<img>`, // an img with no attributes at all
		`<mbp:pagebreak/><p>a</p>`,
	} {
		got := pictures([]byte(in), func(int) (string, bool) { return "", false })
		if string(got) != in {
			t.Errorf("\n in %q\nout %q", in, got)
		}
	}
}

func TestAnImgTagKeepsWhatAPictureNeeds(t *testing.T) {
	in := `<img recindex="00001" alt="a cat" title="Fig. 1" width="100" height="50" align="center" class="calibre10" aid="3Q286"/>`
	got := string(pictures([]byte(in), func(rec int) (string, bool) { return "data:image/jpeg;base64,AAAA", rec == 1 }))
	for _, want := range []string{`src="data:image/jpeg;base64,AAAA"`, `alt="a cat"`, `title="Fig. 1"`, `width="100"`, `height="50"`, `align="center"`} {
		if !strings.Contains(got, want) {
			t.Errorf("%s is missing from %q", want, got)
		}
	}
	// ⛔ And the producer's own bookkeeping is NOT kept. A class naming a
	// stylesheet this reader does not have, and an aid that is a KF8 internal
	// identifier, are noise in a document model.
	for _, gone := range []string{"calibre10", "3Q286"} {
		if strings.Contains(got, gone) {
			t.Errorf("%s survived in %q", gone, got)
		}
	}
}

// ⛔ A TOKENISER ERROR RETURNS WHAT WAS READ rather than refusing: the HTML
// reader that follows is the one entitled to an opinion on malformed markup,
// and half a book is better than none.
func TestMarkupTheTokeniserGivesUpOn(t *testing.T) {
	// A tag name of nothing, then a very deep nesting: whatever the tokeniser
	// makes of it, the text in front of it has to survive.
	in := []byte("<p>kept</p><" + strings.Repeat("<", 100))
	got := pictures(in, func(int) (string, bool) { return "", false })
	if !bytes.Contains(got, []byte("kept")) {
		t.Errorf("the text before the rubbish was lost: %q", got)
	}
}

// ⛔ A BOOK WHOSE HEADER NAMES NO FIRST IMAGE RECORD. Zero is a valid record
// number, so absence is 0xFFFFFFFF and not zero — and a book with no pictures
// that still has an <img> in its text (producers leave them behind) must drop
// the tag rather than read record 0xFFFFFFFE.
func TestAPictureInABookThatHasNone(t *testing.T) {
	h := header0{
		compression: compressionNone,
		textLength:  uint32(len(`<p>a<img recindex="00001"/>b</p>`)),
		textRecords: 1,
		recordSize:  4096,
		encoding:    encodingUTF8,
		mobiType:    mobiTypeBook,
		// firstImage is left out, so the field holds zero in the bytes and
		// the reader keeps it as absent.
		headerLength: 72,
	}
	b := newBook()
	b.records = [][]byte{h.bytes(), []byte(`<p>a<img recindex="00001"/>b</p>`), jpegBytes}
	doc, err := Parse(b.bytes(), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if urls := imageURLs(doc.Blocks); len(urls) != 0 {
		t.Errorf("pictures came out of a book with no image records: %.40q", urls)
	}
	if got := plainText(doc.Blocks); !strings.Contains(got, "a") || !strings.Contains(got, "b") {
		t.Errorf("the text around the dropped picture is %q", got)
	}
}

// ⛔ And an <img> with neither src nor recindex, which is how the resolver's
// own "that is not a picture number" branch is reached now that the caller no
// longer second-guesses it.
func TestAnImgTagThatNamesNothing(t *testing.T) {
	src := bookWithPictures(t, `<p>before<img alt="nothing"/>after</p>`, jpegBytes)
	doc, err := Parse(src, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if urls := imageURLs(doc.Blocks); len(urls) != 0 {
		t.Errorf("a picture came from a tag that names none: %.40q", urls)
	}
}
