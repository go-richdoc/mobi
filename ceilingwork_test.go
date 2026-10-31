// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

// Does a ceiling stop the WORK, or only the ANSWER?
//
// ⛔ THE CONTROL IS THE WHOLE TEST, ported from go-richdoc/data — where it found
// a JSON ceiling checked after encoding/json had built everything — and from
// go-richdoc/html, where it found a refusal that cost 96 MB and now costs
// 7 096 bytes, and from go-richdoc/epub, where it found a node ceiling that
// bounded each chapter and not the book. Every one of those read as correct. So
// it is run here rather than argued about, and it stays committed so that a
// change to how records are inflated has to face it.
//
// ⛔ Every figure is a count of BYTES from runtime.MemStats: exact, and
// independent of what else the machine is doing (taken at a load average above
// 50). There is no timing claim here.
//
// ⛔ AND THE INPUT IS BUILT OUTSIDE THE MEASUREMENT. In the sibling package the
// first draft of this control converted a string to bytes INSIDE the measured
// closure and reported a ratio of 7.3 — a defect that did not exist, being the
// copy and nothing else.
package mobi

import (
	"runtime"
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
)

func allocatedBy(t *testing.T, f func() (*richdoc.Document, error)) (total uint64, retained int64, err error) {
	t.Helper()
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	doc, err := f()

	runtime.ReadMemStats(&after)
	total = after.TotalAlloc - before.TotalAlloc
	runtime.GC()
	var held runtime.MemStats
	runtime.ReadMemStats(&held)
	// ⛔ SIGNED: a refusal frees what it inflated, so the heap can end lower
	// than it started, and as a uint64 subtraction that reads as eighteen
	// quintillion rather than as minus four hundred kilobytes.
	retained = int64(held.HeapAlloc) - int64(before.HeapAlloc)
	runtime.KeepAlive(doc)
	return total, retained, err
}

// TestTheTextByteCeilingStopsTheWork.
//
// ⛔ Two books refused by the SAME ceiling, the second holding eight times the
// text of the first. The reader was supposed to stop at the same inflated byte
// in both, so the costs must agree. If allocation tracks the book instead, the
// ceiling is checked after the inflation it exists to prevent.
func TestTheTextByteCeilingStopsTheWork(t *testing.T) {
	const (
		ceiling = 4 << 10
		small   = 20_000  // ~200 KB of text
		large   = 160_000 // eight times as much
	)

	cost := func(n int) uint64 {
		src := plainBook(t, "<html><body>"+strings.Repeat("<p>one</p>", n)+"</body></html>",
			header0{title: "A Book"})
		total, _, err := allocatedBy(t, func() (*richdoc.Document, error) {
			return Parse(src, Limits{MaxTextBytes: ceiling})
		})
		if err == nil {
			t.Fatalf("%d paragraphs against a %d byte ceiling was accepted", n, ceiling)
		}
		t.Logf("%d paragraphs, book %d bytes, refused: %d allocated", n, len(src), total)
		return total
	}

	s, l := cost(small), cost(large)
	ratio := float64(l) / float64(s)
	t.Logf("same ceiling, 8x the text: %d -> %d bytes allocated, ratio %.1f", s, l, ratio)
	if ratio > 2 {
		t.Errorf("the text-byte ceiling does not bound the work: 8x the text costs %.1fx "+
			"the allocation", ratio)
	}
}

// TestWhatOneByteOfBookBuys, for the books that are accepted.
func TestWhatOneByteOfBookBuys(t *testing.T) {
	const (
		paragraphs = 20_000
		ceiling    = 400 // bytes allocated per byte of book
	)
	src := plainBook(t, "<html><body>"+strings.Repeat("<p>one</p>", paragraphs)+"</body></html>",
		header0{title: "A Book"})
	total, retained, err := allocatedBy(t, func() (*richdoc.Document, error) {
		return Parse(src, Limits{})
	})
	if err != nil {
		t.Fatalf("%d bytes: %v", len(src), err)
	}
	perByte := float64(total) / float64(len(src))
	t.Logf("%d bytes of book -> %d allocated (%.1f per byte), %d retained",
		len(src), total, perByte, retained)
	if perByte > ceiling {
		t.Errorf("allocates %.1f bytes per byte of book, past the ceiling of %d",
			perByte, ceiling)
	}
}
