// Copyright (c) 2026, the go-richdoc/mobi authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package mobi

import "testing"

// FuzzParse feeds arbitrary bytes to the reader.
//
// ⛔ WHAT IT ASSERTS IS THAT THERE IS NO THIRD OUTCOME. Every byte string is
// either a document or a named refusal; a panic is neither, and a panic is what
// this format invites — every length, offset, record number and record count it
// uses comes out of the file, and each one of them slices a byte slice.
//
// The seeds are the shapes that were found by hand: a whole small book, a book
// whose record list points outside the file, a title claiming four gigabytes, a
// trailing-entry size that walks backwards through the record, a back-reference
// pointing before the start of the output.
func FuzzParse(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("MOBI"))
	f.Add(plainBook(&testing.T{}, "<p>a book</p>", header0{title: "A Title"}))
	f.Add(bookWithPictures(&testing.T{}, `<p><img recindex="00001"/></p>`, jpegBytes))

	deep := newBook()
	deep.records = [][]byte{
		header0{
			compression: compressionPalmDoc, textRecords: 1, recordSize: 4096,
			textLength: 16, encoding: encodingUTF8, mobiType: mobiTypeBook,
			extraFlags: 0x3,
		}.bytes(),
		{0x80, 0x40, 0x00, 0x00}, // a back-reference into an empty output
	}
	f.Add(deep.bytes())

	f.Fuzz(func(t *testing.T, src []byte) {
		doc, err := Parse(src, Limits{MaxTextBytes: 1 << 20, MaxTotalImageBytes: 1 << 20})
		if err != nil {
			// ⛔ A refusal has to SAY something. An empty error message is a
			// refusal a caller cannot pass on to a person.
			if err.Error() == "" {
				t.Fatal("a refusal with nothing in it")
			}
			return
		}
		if doc == nil {
			t.Fatal("no document and no error")
		}
		// Walking it is part of the assertion: a document that cannot be read
		// back is not a document.
		plainText(doc.Blocks)
		imageURLs(doc.Blocks)
	})
}
