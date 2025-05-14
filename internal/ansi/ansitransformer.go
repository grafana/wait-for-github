package ansi

import (
	"iter"
	"slices"

	"golang.org/x/text/transform"
)

const ESC = 0x1b

// SegmentType represents the type of segment found in the input
type SegmentType int

const (
	// PlainText is a regular text segment that should be transformed
	PlainText SegmentType = iota
	// AnsiSequence is a complete ANSI sequence that should be preserved as-is
	AnsiSequence
	// PartialAnsiSequence is an incomplete ANSI sequence that needs more input
	PartialAnsiSequence
)

// Segment represents a ANSI-sequence-separated segment of the input. The
// elements `[Start, End)` are the longest contiguous slice of elements which
// have type `Type`.
type Segment struct {
	// What kind of segment this is
	Type SegmentType
	// Start position in the original input
	Start int
	// End position in the original input (exclusive)
	End int
}

// `ANSITransformer` wraps an inner Transformer, preserving ANSI SGR escape
// sequences (`ESC [ ... m`). What does this mean in plain English? If you have
// a string containing SGR colour codes (for the terminal), like `\x1b[31mhello
// world\x1b[0m` and you want to title case it, you could use `cases.Title`,
// except that this doesn't know anything about ANSI sequences, so it's likely
// to mess the string up, or at least not give the expected result.
// `cases.Titles` is a [`Transformer`]. This is an interface that expresses the
// idea of processing byte arrays, and applying transformations to them.
// `ANSITransformer` is a `Transformer` which wraps another `Transformer`. It
// removes any SGR codes, passing the plain text to the wrapped `Transformer`,
// and reassembles the result. So the underlying text is transformed, but any
// colours are preserved.
//
// [`Transformer`]: https://pkg.go.dev/golang.org/x/text/transform#Transformer
type ANSITransformer struct {
	// the transformer to apply to text segments
	inner transform.Transformer

	// bytes from previous calls that couldn't be processed yet
	leftover []byte
}

// NewANSITransformer returns a Transformer that applies inner to text segments
// but leaves ANSI bracket sequences intact.
func NewANSITransformer(inner transform.Transformer) *ANSITransformer {
	t := &ANSITransformer{inner: inner}

	return t
}

// isStartOfSGRAnsiSequence examines data starting at the beginning of src, looking for
// an SGR ANSI sequence.
//
// Returns:
// - seqLen: length of complete or partial ANSI sequence (0 if not a valid sequence)
// - isPartial: true if this could be a valid ANSI sequence, but it's incomplete
func isStartOfSGRAnsiSequence(src []byte) (seqLen int, isPartial bool) {
	// Single ESC at end could be start of sequence
	if len(src) == 1 && src[0] == ESC {
		return 1, true
	}

	// Not enough bytes to start a sequence
	if len(src) < 2 {
		return 0, false
	}

	if src[0] != ESC || src[1] != '[' {
		return 0, false
	}

	// We have at least ESC [ - now look for the end or invalid chars
	i := 2
	for ; i < len(src); i++ {
		b := src[i]
		if b == 'm' {
			// Complete sequence found
			return i + 1, false
		}

		if (b < '0' || b > '9') && b != ';' {
			// Invalid character for ANSI sequence
			return 0, false
		}
	}

	// If we reached here without finding 'm', it's a valid partial sequence
	return len(src), true
}

// segments iterates through the input, yielding segments of different types
func (w *ANSITransformer) segments(src []byte) iter.Seq[Segment] {
	return func(yield func(Segment) bool) {
		textStart := 0
		i := 0

		for i < len(src) {
			// Try to identify an ANSI sequence at the current position
			seqLen, isPartial := isStartOfSGRAnsiSequence(src[i:])

			if seqLen == 0 {
				// Not the start of a sequence, move to next byte
				i++
				continue
			}

			// We found either a complete or partial sequence

			// If there's text before this sequence, yield it as PlainText
			if i > textStart {
				if !(yield(Segment{
					Type:  PlainText,
					Start: textStart,
					End:   i,
				})) {
					return
				}
			}

			if isPartial {
				// Otherwise, yield it as a partial sequence and stop
				yield(Segment{
					Type:  PartialAnsiSequence,
					Start: i,
					End:   len(src),
				})

				// No more processing after a partial sequence
				return
			}

			// Complete sequence
			if !(yield(Segment{
				Type:  AnsiSequence,
				Start: i,
				End:   i + seqLen,
			})) {
				return
			}

			// Move past the sequence
			i += seqLen
			textStart = i
		}

		// There are no more ANSI sequences, but we have some text left at the end
		// of our buffer to yield.
		if textStart < len(src) {
			if !(yield(Segment{
				Type:  PlainText,
				Start: textStart,
				End:   len(src),
			})) {
				return
			}
		}
	}
}

// transformText applies the inner transformer to a text segment.
func (w *ANSITransformer) transformText(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	if len(src) == 0 {
		return 0, 0, nil
	}

	return w.inner.Transform(dst, src, atEOF)
}

// Transform processes `src` into `dst`, preserving complete ANSI sequences and
// applying `inner` to the text content.
func (w *ANSITransformer) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	if len(w.leftover) > 0 {
		src = slices.Concat(w.leftover, src)
		w.leftover = w.leftover[:0]
	}

	// Process each segment from the iterator
	for segment := range w.segments(src) {
		switch segment.Type {
		case PlainText:
			// Transform this text segment
			textChunk := src[segment.Start:segment.End]
			innerAtEOF := atEOF && segment.End == len(src)

			nTransformedDst, nConsumedByInner, err := w.transformText(dst[nDst:], textChunk, innerAtEOF)
			nDst += nTransformedDst

			if err != nil {
				// Save the rest for next time
				processed := segment.Start + nConsumedByInner
				w.leftover = append(w.leftover, src[processed:]...)
				src = src[0:]

				return nDst, processed, err
			}

		case AnsiSequence:
			// Copy ANSI sequence to our output directly
			seqLen := segment.End - segment.Start
			if nDst+seqLen > len(dst) {
				w.leftover = append(w.leftover, src[segment.Start:]...)
				return nDst, segment.Start, transform.ErrShortDst
			}

			copy(dst[nDst:], src[segment.Start:segment.End])
			nDst += seqLen

		case PartialAnsiSequence:
			if !atEOF {
				// Save the partial sequence for next time
				w.leftover = append(w.leftover, src[segment.Start:segment.End]...)
			}
			return nDst, segment.Start, nil
		}

		nSrc = segment.End
	}

	return nDst, nSrc, nil
}

// Span implements the `transform.SpanningTransformer` interface. It returns the
// length of the longest prefix of src that consists of either ANSI sequences
// (which don't need transformation) or spans that the inner transformer (if
// it's a `SpanningTransformer`) reports don't need transformation.
func (w *ANSITransformer) Span(src []byte, atEOF bool) (n int, err error) {
	if len(w.leftover) > 0 {
		// If we have leftover bytes, we need to transform
		return 0, nil
	}

	spanEnd := 0

	// Process each segment from the iterator
	for segment := range w.segments(src) {
		switch segment.Type {
		case PlainText:
			innerAtEOF := atEOF && segment.End == len(src)

			// Without a spanning inner transformer, assume all text needs transformation
			spanningInner, ok := w.inner.(transform.SpanningTransformer)
			if !ok {
				return spanEnd, nil
			}

			// If we have a spanning inner transformer, check if it needs to transform this text
			textChunk := src[segment.Start:segment.End]

			innerSpan, err := spanningInner.Span(textChunk, innerAtEOF)
			if err != nil && err != transform.ErrEndOfSpan {
				return spanEnd, err
			}

			if innerSpan < len(textChunk) {
				// The inner transformer would transform something in this span. So the
				// non-transformable region ends at spanEnd.
				return spanEnd, nil
			}

			// This text segment doesn't need transformation
			spanEnd = segment.End

		case AnsiSequence:
			// ANSI sequences don't need transformation
			spanEnd = segment.End

		case PartialAnsiSequence:
			// Can't determine if partial sequences need transformation
			return spanEnd, nil
		}
	}

	// If we get here, the entire src doesn't need transformation
	return len(src), nil
}

// Reset clears the transformer's state.
func (w *ANSITransformer) Reset() {
	w.leftover = w.leftover[:0]
	w.inner.Reset()
}
