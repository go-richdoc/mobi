// Copyright (c) 2026, the go-richdoc/mobi authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

// Package mobi reads a Kindle MOBI ebook into a [richdoc.Document].
//
// A MOBI file is three formats stacked on each other, and reading one is
// reading all three:
//
//   - a Palm Database (PDB): a 78-byte header and a list of byte offsets, one
//     per record. Everything else lives inside those records.
//   - record 0, which holds a PalmDOC header, a MOBI header and usually an
//     EXTH header of metadata.
//   - the book itself, which is ONE HTML document split across records 1..n
//     and compressed, with its pictures in records of their own, referred to
//     by number rather than by name.
//
// The HTML then goes through [github.com/go-richdoc/html], exactly as an EPUB's
// chapters do, so a Kindle book and an EPUB come out as the same kind of
// document and convert through the same writers.
//
// # What it reads, and what it says no to
//
// It reads MOBI 6 — mobi type 2, uncompressed or PalmDOC-compressed, in CP1252
// or UTF-8 — which is what the format means in practice: the file this was
// measured against is Project Gutenberg's own Pride and Prejudice, 25 MB, 440
// records, 259 of text and 165 JPEGs, produced by Calibre 9.5.0.
//
// Three things are REFUSED BY NAME rather than half-read, because each one
// would otherwise come out as a book with its text missing:
//
//   - DRM (encryption type 1 or 2). The bytes are encrypted to a device key
//     this package will never have. There is nothing to attempt.
//   - HUFF/CDIC compression (type 17480), a proprietary Huffman scheme whose
//     only published description is a reference implementation. Rare outside
//     commercial Mobipocket titles.
//   - KF8 (mobi type 248), the AZW3 format. Its text is not one HTML document
//     but a skeleton with fragments woven into it, which is a different reader
//     and not a flag on this one. A COMBINED file — MOBI 6 and KF8 in one
//     container, which is what Calibre writes by default — reads fine: record
//     0 describes the MOBI 6 half, and that is the half this reads.
//
// ⛔ A refusal says which of these it is. "mobi: cannot read this file" would
// send somebody looking for a defect in their book, and three of these four
// files are not defective at all.
//
// # Links inside the book
//
// MOBI's internal links are byte offsets into the decompressed text —
// `<a filepos="0000012345">` — and not names. They come out as PLAIN TEXT,
// because the HTML reader treats an anchor with neither href nor id as
// content, and a link to nowhere is worse than no link: it looks clickable
// and goes nowhere. The text of the link is kept in every case.
package mobi
