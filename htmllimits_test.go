// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package mobi

import (
	"math"
	"strings"
	"testing"

	"github.com/go-richdoc/html"
)

// TestTheInputCeilingCoversWhatIsActuallyHandedOver.
//
// ⛔ THE DEFECT THIS TEST EXISTS FOR: the HTML reader is handed the text with
// every picture substituted as a data URL, so a ceiling of MaxTextBytes alone
// would refuse a legitimate illustrated book. A book at both of this package's
// ceilings must be inside the one handed down.
func TestTheInputCeilingCoversWhatIsActuallyHandedOver(t *testing.T) {
	d := Limits{}.orDefaults()
	got := htmlLimits(d)

	// Base64 is four bytes for every three, so the pictures alone reach this
	// much of the document.
	asDataURLs := d.MaxTotalImageBytes / 3 * 4
	atBothCeilings := d.MaxTextBytes + asDataURLs

	if int64(got.MaxInputBytes) < atBothCeilings {
		t.Errorf("a book at both ceilings hands over %d bytes and the HTML reader is "+
			"given a ceiling of %d — an illustrated book would be refused at a limit "+
			"nobody set", atBothCeilings, got.MaxInputBytes)
	}
	t.Logf("text %d + pictures as data URLs %d = %d handed over, ceiling %d",
		d.MaxTextBytes, asDataURLs, atBothCeilings, got.MaxInputBytes)

	// ⛔ And the planted negative: MaxTextBytes alone is NOT enough, which is
	// what the first draft of htmlLimits passed. If this ever stops being true
	// the comment explaining the sum has gone stale.
	if int64(d.MaxTextBytes) >= atBothCeilings {
		t.Error("MaxTextBytes alone would have covered it, so the sum needs no explaining " +
			"and the comment on htmlLimits is now wrong")
	}
}

// TestTheDepthAndNodeCeilingsArePassedThrough, unchanged by the above.
func TestTheDepthAndNodeCeilingsArePassedThrough(t *testing.T) {
	got := htmlLimits(Limits{MaxDepth: 7, MaxNodes: 11, MaxTextBytes: 300, MaxTotalImageBytes: 100})
	want := html.Limits{MaxDepth: 7, MaxNodes: 11, MaxInputBytes: 500}
	if got != want {
		t.Errorf("htmlLimits gave %+v, want %+v", got, want)
	}
}

// TestTheClampIsNotAConversion.
//
// ⛔ The planted positive is the value that becomes a SMALL POSITIVE int on a
// 32-bit target. A plain conversion overflows, and zero means "use the default"
// to every orDefaults in this family — so it would not look like an error, it
// would look like a caller's enormous ceiling being quietly ignored.
func TestTheClampIsNotAConversion(t *testing.T) {
	got := htmlLimits(Limits{MaxTextBytes: math.MaxInt64 / 2, MaxTotalImageBytes: math.MaxInt64 / 2})
	if got.MaxInputBytes != maxInt {
		t.Errorf("a ceiling past the largest int came through as %d, want %d",
			got.MaxInputBytes, maxInt)
	}
	if got.MaxInputBytes <= 0 {
		t.Error("it came through as zero or negative, which every orDefaults in this " +
			"family reads as 'use the default'")
	}
}

// TestAnIllustratedBookStillParses is the behaviour behind the mapping, at a
// size a test can afford: the pictures make the document bigger than the text,
// and the ceiling handed down has to have room for them.
func TestAnIllustratedBookStillParses(t *testing.T) {
	pic := []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("x", 4_000))
	src := bookWithPictures(t, `<p>one</p><img recindex="00001"/>`, pic)

	// ⛔ A text ceiling that the TEXT fits inside and the DOCUMENT does not:
	// 4 000 bytes of picture become about 5 336 of base64. With MaxTextBytes
	// alone handed down, this is the book that was refused.
	_, err := Parse(src, Limits{MaxTextBytes: 200, MaxTotalImageBytes: 1 << 20})
	if err != nil {
		t.Fatalf("an illustrated book whose text fits and whose data URLs do not: %v", err)
	}
}
