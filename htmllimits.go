// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package mobi

import (
	"math"

	"github.com/go-richdoc/html"
)

// htmlLimits is what this reader's ceilings mean to the HTML reader underneath.
//
// ⛔ WHAT IS HANDED OVER IS NOT THE TEXT. It is the text with every picture
// substituted as a data URL, so the bytes the HTML reader sees are bounded by
// MaxTextBytes PLUS the image budget — and threading MaxTextBytes alone would
// have refused a legitimate illustrated book at a ceiling nobody set. Caught
// while writing the comment that claimed MaxTextBytes was "the right number":
// it is the right number for the text and the wrong one for the document.
//
// ⛔ Twice the image budget, deliberately generous rather than tight. Base64 is
// four bytes for every three, and each picture also carries a
// "data:image/jpeg;base64," prefix whose total depends on how many pictures
// there are — a number this function does not have. An input ceiling that is
// too LARGE here costs nothing, because what actually bounds this reader is
// MaxTextBytes and MaxTotalImageBytes, both already spent by the time the HTML
// reader is called; an input ceiling that is too SMALL refuses real books. So
// the error is made in the harmless direction, on purpose and in writing.
//
// ⛔ CLAMPED via [maxInt], because these are int64 and MaxInputBytes is an int:
// on a 32-bit target or js/wasm a caller's 8 GiB becomes a small and welcoming
// number if it is merely converted. maxInt is declared in text.go, where the
// same conversion is already made for the same reason.
//
// ⛔ AND THE SUM SATURATES, because clamping AFTER adding is too late. The
// first draft was `min(text + 2*images, maxInt)`: with both ceilings near
// MaxInt64 the addition wrapped and the clamp then faithfully preserved
// -4611686018427387907, which html.Limits.orDefaults reads as "use the
// default". A bound that overflows into a smaller bound is the one shape of
// integer bug that looks like no bug at all. Its own test found it.
func htmlLimits(lim Limits) html.Limits {
	// Negatives mean "the default" everywhere in this family, so they are not
	// budget to add up.
	text := max(lim.MaxTextBytes, 0)
	images := max(lim.MaxTotalImageBytes, 0)
	handedOver := satAdd(text, satAdd(images, images))
	return html.Limits{
		MaxDepth:      lim.MaxDepth,
		MaxNodes:      lim.MaxNodes,
		MaxInputBytes: int(min(handedOver, int64(maxInt))),
	}
}

// satAdd is a+b for non-negative a and b, or the largest int64 if that would
// wrap.
func satAdd(a, b int64) int64 {
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}
