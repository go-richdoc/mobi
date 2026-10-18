# mobi

[![CI](https://github.com/go-richdoc/mobi/actions/workflows/ci.yml/badge.svg)](https://github.com/go-richdoc/mobi/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/go-richdoc/mobi.svg)](https://pkg.go.dev/github.com/go-richdoc/mobi)
[![Go Report Card](https://goreportcard.com/badge/github.com/go-richdoc/mobi)](https://goreportcard.com/report/github.com/go-richdoc/mobi)
[![License: BSD-3-Clause](https://img.shields.io/badge/License-BSD--3--Clause-blue.svg)](LICENSE)
[![Coverage](https://img.shields.io/badge/coverage-100%25-brightgreen.svg)](#testing)

A Kindle **MOBI** ebook as a [richdoc](https://github.com/go-richdoc/richdoc)
document, in pure Go. Nothing outside the standard library and
`golang.org/x/net/html` is used, so this builds for `js/wasm` and every
architecture the fleet targets.

```go
doc, err := mobi.Parse(src, mobi.Limits{})
// doc.Blocks is the book; doc.Meta has "title", "author", "language", …
```

From there it is a PDF through [go-pdfkit](https://github.com/go-pdfkit),
Markdown, HTML or DOCX through the rest of the family — a MOBI and an EPUB come
out as the same kind of document and go through the same writers.

## Three formats stacked on each other

Reading a MOBI is reading all three:

| | |
|---|---|
| a **Palm Database** (PDB) | a 78-byte header and a list of byte offsets, one per record. Everything else lives inside those records. |
| **record 0** | a PalmDOC header, a MOBI header, and usually an EXTH block of metadata. |
| the **book** | ONE HTML document split across records 1..n and compressed, with its pictures in records of their own, referred to by number rather than by name. |

The HTML then goes through [go-richdoc/html](https://github.com/go-richdoc/html),
exactly as an EPUB's chapters do.

⛔ **The end of the last record is not written down.** A record entry gives a
start and nothing else, and the format's own documentation does not say how the
last one ends; every reader takes it to be the end of the file, and so does
this one. It is the one bound here that is a convention rather than a field.

## What it reads

MOBI 6 — mobi type 2, uncompressed or PalmDOC-compressed, CP1252 or UTF-8 —
which is what the format means in practice.

⛔ **CP1252 is not Latin-1**, and the difference is the punctuation of every
English book written on Windows. Taken as Latin-1, `0x92` — the apostrophe in
"don't" as Word writes it — becomes U+0092, an invisible control character: the
word comes out with a hole in it, in every contraction in the book. The table
is twenty-seven characters wide and it is in `text.go`.

### Pictures, named two ways

```html
<img recindex="00001"/>                             <!-- Mobipocket's own way -->
<img src="kindle:embed:0001?mime=image/jpeg"/>      <!-- Calibre and KindleGen -->
```

Both resolve to the picture's record and come out as a `data:` URL. ⛔ **The
number in `kindle:embed` is base 32** — digits then capitals, `0-9A-V` — so
`003C` is 108 and `0010` is 32. Read as hexadecimal it would be wrong for every
picture past the thirty-first **and would still look right for the first nine**,
which is the kind of bug that passes a test written on a book with a cover and
nothing else.

⛔ **And it is one-based from the first image record, which was measured rather
than assumed**: of the 164 references in the book below, all 164 land on a JPEG
read one-based and 163 do read zero-based. The one that differs is what makes
the question answerable.

A picture whose record is missing, or whose record turns out not to be a picture
at all, is **dropped** — the records after the text hold indexes, a FLIS and a
FCIS block and the end-of-file marker as well as the pictures — and the text
around it is kept. Losing a picture must never lose a word.

### Links inside the book

MOBI's internal links are byte offsets into the decompressed text —
`<a filepos="0000012345">` — and not names. They come out as **plain text**: a
link that looks clickable and goes nowhere is worse than no link. The words are
kept in every case.

## What it says no to, by name

```
mobi: "Bought From A Shop" is DRM'd (encryption type 2), and the text is
      encrypted to a device key: there is nothing here to read
mobi: "A Commercial Title" is HUFF/CDIC-compressed (2 records from 300),
      which this reader does not decode
mobi: "An AZW3" is a KF8 (AZW3) book: its text is a skeleton with fragments
      woven into it, not one HTML document, and that is a different reader
```

⛔ **A refusal says which of these it is.** "Cannot read this file" would send
somebody looking for a defect in their book, and none of these three files is
defective.

⛔ **And none of them is `ErrFormat`.** That error means "not a Mobipocket file
at all", and a caller sniffing formats by trying readers in turn must not be
told "not mine" about a file that is.

A **combined** file — MOBI 6 and KF8 in one container, which is what Calibre
writes by default — reads fine: record 0 describes the MOBI 6 half, and that is
the half this reads.

## Limits

Every field this format uses to size an allocation is a number *in the file*. It
costs the author of that file nothing to claim four gigabytes of text.

```go
mobi.Limits{
    MaxRecords:         65535,     // the Palm database's own maximum
    MaxTextBytes:       64 << 20,  // decompressed, across every record
    MaxImageBytes:      32 << 20,  // one picture
    MaxTotalImageBytes: 256 << 20, // all of them
    MaxDepth:           0,         // handed to the HTML reader
    MaxNodes:           0,
}
```

The zero value of each means the default above.

## The trap of this format

The bytes at the end of a text record **are not text**, and they are not
compressed either: they are indexing data and, for a multibyte character split
across two records, the overlap. Which entries are present is a bitfield at
offset 240 of record 0, each entry ends with its own size written **backwards**
as a variable-width integer that counts itself, and the lowest bit is different
again — its count is two bits of the last byte.

⛔ Feeding those bytes to the decompressor does not fail. It produces plausible
rubbish at the end of every record — wrong prose, which is the worst kind of
failure a reader has. The book measured below sets flags `0x3` on all 259 of
its text records.

## Testing

100% of statements, measured as the **profile** and not the percentage, plus a
`FuzzParse` target run for a minute on every pull request: 16.3 million
executions in the session that wrote it, no panic.

⛔ **The fixtures are built by hand, field by field, and that is not enough.** A
test that reads a book this package wrote only says that the two halves of one
misreading agree. So the bytes of each PalmDOC case are typed from the format's
own description — and the first version of one of them had its back-reference
distance one short, which read `"Hello ello"`: exactly what a reader with an
off-by-one produces on a real book.

### The real measurement

Beside every `book.mobi`, `TestARealCorpus` uses a `book.txt` if there is one and
reports how much of it came out. Project Gutenberg publishes both formats of
every title it has:

```sh
curl -o pg1342.mobi https://www.gutenberg.org/ebooks/1342.kf8.images
curl -o pg1342.txt  https://www.gutenberg.org/ebooks/1342.txt.utf-8
MOBI_CORPUS=. go test -run TestARealCorpus -v ./...
```

*Pride and Prejudice*, 25 MB, 440 records — 259 of text and 165 JPEGs, Calibre
9.5.0 — read on 2026-10-18:

| | reference | with `html` v0.1.0 | with `html` v0.1.2 |
|---|---|---|---|
| words | 126 630 | 125 816 | **126 512** |
| sentences of 60 characters or more | 4 215 | 3 638 (86.3%) | **4 125 (97.9%)** |

164 pictures referenced, **163 carried as bytes**; the remaining one is a
`kindle:flow:` SVG, which lives in the KF8 half.

⛔ **The first column's 13.7% gap was not in this package**, and reading this
book is what found it:
[go-richdoc/html#4](https://github.com/go-richdoc/html/issues/4) dropped the
space following an inline element, so `<i>Walt Whitman</i> has` came out as one
word — under `epub` and `email` too, all along. The third column is the same
book through the one-line fix
(go-richdoc/html v0.1.2).

⛔ **The 90 that still differ are not prose.** They are the book's list of
illustrations and its page-number column, which the two renderings lay out
differently — a vocabulary mismatch, not lost text. A run reporting much worse
than 97% means something here broke.

⛔ **And getting the judge symmetric took three readings.** The first said
76.8%, the second 79.4%, and every "missing" sentence inspected was present: the
plain-text rendering marks italics with underscores, writes
`[Illustration: ...]` where the ebook holds a picture, and writes an em dash as
`--` where the ebook holds a real one. A judge carrying a markup layer the
subject does not have measures the markup.

⛔ **There is no book in `testdata`**, deliberately: a Kindle library is
somebody's own. Without `MOBI_CORPUS` the corpus test **skips and says so**,
because a test that passes by doing nothing is worse than one that is not there.

## Bibliography

The format is not published by its owner. What this was written from:

- [MOBI](https://wiki.mobileread.com/wiki/MOBI) — the record-0 layout, the
  compression types, the EXTH record numbers, the extra-data flags.
- [PDB](https://wiki.mobileread.com/wiki/PDB) — the Palm database header and
  record list.
- [PalmDOC](https://wiki.mobileread.com/wiki/PalmDOC) — the LZ77 scheme, byte
  range by byte range.

## The rest of the family

[richdoc](https://github.com/go-richdoc/richdoc) is the document model;
[html](https://github.com/go-richdoc/html), [epub](https://github.com/go-richdoc/epub),
[fb2](https://github.com/go-richdoc/fb2), [markdown](https://github.com/go-richdoc/markdown)
and [email](https://github.com/go-richdoc/email) are its other readers.

## License

BSD-3-Clause. See [LICENSE](LICENSE).
