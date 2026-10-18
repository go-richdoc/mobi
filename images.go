// Copyright (c) 2026, the go-richdoc/mobi authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package mobi

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// pictures turns every <img recindex="N"> in the book into an ordinary
// <img src="data:...">, so that the HTML reader downstream sees a picture
// where the book meant one.
//
// ⛔ A MOBI REFERS TO ITS PICTURES BY RECORD NUMBER, not by name, and the
// number is in an attribute no HTML parser knows: `recindex`. Parse the book
// first and the reference is gone — the attribute is dropped, the image has no
// src, and what is left is an empty picture nothing can resolve. So the
// rewrite happens on the TOKENS, before anything builds a tree.
//
// ⛔ It is a tokeniser and not a search-and-replace. A MOBI's HTML is machine
// written but it is not tidy: attributes come unquoted, tags come unclosed,
// and `recindex` appears in the text of some books as prose about the format.
// Every token other than an img tag is re-emitted exactly as it was read.
func pictures(doc []byte, resolve func(rec int) (string, bool)) []byte {
	z := html.NewTokenizer(bytes.NewReader(doc))
	var out bytes.Buffer
	out.Grow(len(doc))
	for {
		switch z.Next() {
		case html.ErrorToken:
			// ⛔ Including io.EOF, which is how this ends. The tokeniser's
			// other errors are "this is not HTML enough for me", and what has
			// been rewritten so far is still the book: it is returned rather
			// than refused, because the HTML reader that follows is the one
			// entitled to an opinion on malformed markup.
			return out.Bytes()
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			if string(name) != "img" || !hasAttr {
				out.Write(z.Raw())
				continue
			}
			out.WriteString(imgTag(z, resolve))
		default:
			out.Write(z.Raw())
		}
	}
}

// imgTag rewrites one img tag, keeping the attributes a picture needs.
func imgTag(z *html.Tokenizer, resolve func(rec int) (string, bool)) string {
	var src, keep string
	rec := 0
	for {
		key, val, more := z.TagAttr()
		switch string(key) {
		case "recindex":
			// A MOBI writes it zero-padded: recindex="00001".
			if n, err := strconv.Atoi(strings.TrimSpace(string(val))); err == nil {
				rec = n
			}
		case "src":
			src = string(val)
		case "alt", "title", "width", "height", "align":
			keep += fmt.Sprintf(" %s=%q", key, val)
		}
		if !more {
			break
		}
	}
	// ⛔ TWO WAYS OF NAMING THE SAME THING, and the second is the one real
	// books use. Mobipocket's own way is `recindex="00001"`; Calibre and
	// KindleGen write `src="kindle:embed:0001?mime=image/jpeg"` instead, and
	// the Project Gutenberg book this was measured against uses that one for
	// all 164 of its pictures. A reader that knew only recindex would carry
	// every picture through as an unresolvable URL — not an error, just a
	// book of broken images.
	if n, ok := kindleEmbed(src); ok {
		rec, src = n, ""
	}
	// ⛔ ONE GUARD, in the resolver. This asked `rec > 0` here as well, and
	// the coverage profile is what pointed it out: the resolver's own "not a
	// picture number" branch could not be reached, because this one got there
	// first. Two checks of one thing mean the one that is wrong is the one
	// nothing exercises.
	if src == "" {
		if url, ok := resolve(rec); ok {
			src = url
		}
	}
	if src == "" {
		// ⛔ A picture whose record is missing becomes NOTHING, not an empty
		// img. The reader downstream happens to drop an img with no src as
		// well — measured, not assumed — so this changes no document; what it
		// changes is THIS function's output, which is HTML, and emitting
		// `<img src=""/>` into it would be wrong markup kept alive by
		// somebody else's forgiveness. A mutation sweep survived here for
		// exactly that reason, and the test that holds it reads the bytes.
		return ""
	}
	return fmt.Sprintf("<img src=%q%s/>", src, keep)
}

// kindleEmbed reads the record number out of a "kindle:embed:0001" URL, which
// is how Calibre and KindleGen refer to a picture.
//
// ⛔ THE NUMBER IS BASE 32, in digits and then capitals: 0-9 then A-V, so
// 003C is 108 and 0010 is 32. Read as hexadecimal it would be wrong for every
// picture past the thirty-first and would still LOOK right — 0001 to 0009 are
// the same number in both — which is the kind of bug that passes a test
// written on a book with a cover and nothing else.
//
// ⛔ And it is ONE-BASED from the first image record, which was measured
// rather than assumed: of the 164 references in the Gutenberg book, all 164
// land on a JPEG read one-based and 163 do read zero-based. The one that
// differs is what makes the question answerable.
func kindleEmbed(src string) (int, bool) {
	rest, ok := strings.CutPrefix(src, "kindle:embed:")
	if !ok {
		return 0, false
	}
	if i := strings.IndexAny(rest, "?#"); i >= 0 {
		rest = rest[:i]
	}
	if rest == "" {
		return 0, false
	}
	n := 0
	for _, c := range rest {
		var d int
		switch {
		case c >= '0' && c <= '9':
			d = int(c - '0')
		case c >= 'A' && c <= 'V':
			d = int(c-'A') + 10
		default:
			return 0, false
		}
		// A reference long enough to overflow is not a reference to anything.
		if n > 1<<20 {
			return 0, false
		}
		n = n*32 + d
	}
	return n, true
}

// imageSet reads the picture records and returns a resolver from a book's
// record index to a data URL.
//
// ⛔ The pictures are read LAZILY, one at a time, as the rewrite asks for
// them. A MOBI keeps every picture in the file whether the text shows it or
// not: a cover and a thumbnail at least, which EXTH 201 and 202 point at and
// which no <img> in the text mentions. Converting a book has no use for the
// ones nothing refers to.
func imageSet(c *container, h *header, lim Limits) func(int) (string, bool) {
	cache := map[int]string{}
	var total int64
	return func(rec int) (string, bool) {
		if url, ok := cache[rec]; ok {
			return url, url != ""
		}
		url := ""
		defer func() { cache[rec] = url }()

		if h.firstImage == absent || rec < 1 {
			return "", false
		}
		// The book counts its pictures from one, starting at the record the
		// header names.
		b := c.record(int(h.firstImage) + rec - 1)
		if b == nil || int64(len(b)) > lim.MaxImageBytes || total+int64(len(b)) > lim.MaxTotalImageBytes {
			return "", false
		}
		kind := sniff(b)
		if kind == "" {
			// ⛔ SNIFFED, and dropped when it is not a picture. The records
			// after the text hold indexes, a FLIS and a FCIS block and the
			// end-of-file marker as well as the pictures, and a book whose
			// first-image index is wrong by one would otherwise put an index
			// record into the page as an image.
			return "", false
		}
		total += int64(len(b))
		url = "data:" + kind + ";base64," + base64.StdEncoding.EncodeToString(b)
		return url, true
	}
}

// sniff names the picture format from its first bytes, or "" for anything that
// is not one of the four a MOBI may carry.
func sniff(b []byte) string {
	switch {
	case len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF:
		return "image/jpeg"
	case len(b) >= 8 && string(b[:8]) == "\x89PNG\r\n\x1a\n":
		return "image/png"
	case len(b) >= 6 && (string(b[:6]) == "GIF87a" || string(b[:6]) == "GIF89a"):
		return "image/gif"
	case len(b) >= 2 && b[0] == 'B' && b[1] == 'M':
		return "image/bmp"
	}
	return ""
}
