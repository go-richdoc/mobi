// Copyright (c) 2026, the go-richdoc/mobi authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package mobi

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

// TestARealCorpus reads every .mobi in a directory and says what came out.
//
// ⛔ THERE IS NO BOOK IN testdata, and that is deliberate rather than lazy. A
// Kindle library is somebody's own — bought, personal, and in most cases not
// redistributable — so a fixture this repository may carry has to be one built
// by hand, and the real population is read from wherever the person running
// the test keeps theirs:
//
//	MOBI_CORPUS=~/Books go test -run TestARealCorpus -v ./...
//
// Without the variable the test SKIPS and says so, because a test that passes
// by doing nothing is worse than one that is not there.
//
// ⛔ AND IT LOOKS FOR A SECOND RENDERING. Beside every book.mobi it will use a
// book.txt if there is one, and report how many of that file's sentences come
// out of ours. This is the only question a fixture cannot answer: a test built
// on a book this package wrote only says that the two halves of one misreading
// agree. Project Gutenberg publishes both formats of every title it has, which
// is where the pair this was developed against came from:
//
//	curl -o pg1342.mobi https://www.gutenberg.org/ebooks/1342.kf8.images
//	curl -o pg1342.txt  https://www.gutenberg.org/ebooks/1342.txt.utf-8
//
// Measured on that pair, 2026-10-18: 125 816 words against the reference's
// 126 630, and 3 638 of 4 215 sentences of sixty characters or more found
// verbatim (86.3%). The gap is NOT in this package — it is
// github.com/go-richdoc/html issue 4, where a space following an inline
// element is dropped, so "<i>Walt Whitman</i> has" comes out as one word. Any
// run of this that reports much better than 86% means that issue is fixed, and
// one that reports much worse means something here broke.
//
// What it asserts is what a population can assert without knowing the books:
// that every one of them opens or refuses FOR A NAMED REASON, that what opens
// has prose in it, and that none of them panics or hangs. The numbers are
// printed rather than asserted — a population measures the reader, and an
// expected figure would measure the library.
func TestARealCorpus(t *testing.T) {
	dir := os.Getenv("MOBI_CORPUS")
	if dir == "" {
		t.Skip("MOBI_CORPUS is not set: no real books to read. Set it to a directory of .mobi files.")
	}
	if strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatal(err)
		}
		dir = filepath.Join(home, dir[2:])
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var read, refused int
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !isMobiName(name) {
			continue
		}
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			doc, err := Parse(src, Limits{})
			if err != nil {
				// ⛔ A refusal is a RESULT here and not a failure: DRM,
				// HUFF/CDIC and KF8 are the three this reader says no to, and
				// a real library holds all three. What is checked is that the
				// refusal says which.
				refused++
				t.Logf("refused: %v", err)
				if strings.Contains(err.Error(), "cannot read this file") {
					t.Errorf("the refusal does not say what is wrong: %v", err)
				}
				return
			}
			read++
			text := plainText(doc.Blocks)
			words := len(strings.Fields(text))
			t.Logf("%d blocks, %d pictures, %d words, title %q",
				len(doc.Blocks), len(imageURLs(doc.Blocks)), words, doc.Meta["title"])
			if words < 10 {
				t.Errorf("a book that opened and came out with %d words in it", words)
			}

			ref := filepath.Join(dir, strings.TrimSuffix(name, filepath.Ext(name))+".txt")
			b, err := os.ReadFile(ref)
			if err != nil {
				return
			}
			found, all := sentencesInCommon(string(b), text)
			t.Logf("against %s: %d of %d sentences of sixty characters or more (%.1f%%), %d words against %d",
				filepath.Base(ref), found, all, 100*float64(found)/float64(all),
				words, len(strings.Fields(string(b))))
		})
	}
	t.Logf("%d books read, %d refused", read, refused)
}

func isMobiName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mobi", ".azw", ".prc":
		return true
	}
	return false
}

var (
	corpusSentence     = regexp.MustCompile(`[^.!?]{60,400}[.!?]`)
	corpusIllustration = regexp.MustCompile(`\[Illustration[^\]]*\]`)
	corpusDashes       = regexp.MustCompile(`-+`)
)

// sentencesInCommon says how many of the reference's long sentences are in
// ours, word for word.
//
// ⛔ BOTH SIDES ARE STRIPPED ALIKE, and getting that wrong cost two false
// readings while this was being developed. A plain-text rendering marks
// italics with underscores, writes "[Illustration: ...]" where the ebook holds
// a picture, and writes an em dash as "--" where the ebook holds a real one.
// Each of those made hundreds of sentences look missing that were present. A
// judge that carries a markup layer the subject does not measures the markup.
func sentencesInCommon(reference, ours string) (found, all int) {
	norm := func(s string) string {
		s = corpusIllustration.ReplaceAllString(s, " ")
		s = strings.ReplaceAll(s, "_", "")
		for from, to := range map[string]string{"’": "'", "‘": "'", "“": `"`, "”": `"`, "—": "-", "–": "-"} {
			s = strings.ReplaceAll(s, from, to)
		}
		s = corpusDashes.ReplaceAllString(s, "-")
		return strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ")
	}
	r, g := norm(reference), norm(ours)
	// ⛔ SENTENCES, not words: a word count agrees with a shuffled book.
	// Sixty characters at least, so that "Chapter 1." cannot match by
	// accident.
	for _, s := range corpusSentence.FindAllString(r, -1) {
		all++
		if strings.Contains(g, s) {
			found++
		}
	}
	return found, all
}
