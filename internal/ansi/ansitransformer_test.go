package ansi

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/transform"
)

func TestTransform(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no ANSI",
			input: "hello world",
			want:  "Hello World",
		},
		{
			name:  "empty input",
			input: "",
			want:  "",
		},
		{
			name:  "only ANSI escape",
			input: "\x1b[31m\x1b[0m",
			want:  "\x1b[31m\x1b[0m",
		},
		{
			name:  "partial ANSI at EOF",
			input: "\x1b[31hello",
			want:  "\x1b[31Hello",
		},
		{
			name:  "non-ASCII text",
			input: "héllo \x1b[34mwörld\x1b[0m",
			want:  "Héllo \x1b[34mWörld\x1b[0m",
		},
		{
			name:  "single ANSI segment",
			input: "\x1b[31mfoo bar\x1b[0m",
			want:  "\x1b[31mFoo Bar\x1b[0m",
		},
		{
			name:  "single ANSI segment with prefix, no space",
			input: "bar\x1b[31mfoo\x1b[0m",
			want:  "Bar\x1b[31mfoo\x1b[0m",
		},
		{
			name:  "single ANSI segment with suffix",
			input: "\x1b[31mfoo\x1b[0m bar",
			want:  "\x1b[31mFoo\x1b[0m Bar",
		},
		{
			name:  "multiple ANSI segments",
			input: "foo \x1b[32mbar \x1b[0mbaz",
			want:  "Foo \x1b[32mBar \x1b[0mBaz",
		},
		{
			name:  "incomplete ANSI at EOF",
			input: "\x1b[33(hello",
			want:  "\x1b[33(Hello",
		},
		{
			name:  "unicode in text",
			input: "héllo \x1b[34mwörld\x1b[0m",
			want:  "Héllo \x1b[34mWörld\x1b[0m",
		},
		{
			name:  "invalid SGR (letter in params)",
			input: "pre\x1b[3amfoo\x1b[0mpost",
			want:  "Pre\x1b[3Amfoo\x1b[0mpost",
		},
		{
			name:  "invalid SGR (punctuation in params)",
			input: "\x1b[1;2?mhello WORLD",
			want:  "\x1b[1;2?Mhello World",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			titlecase := cases.Title(language.English)

			tr := NewANSITransformer(titlecase)
			out, _, err := transform.String(tr, tc.input)
			require.NoError(t, err)
			require.Equal(t, tc.want, out)
		})
	}
}

func TestBytes(t *testing.T) {
	titlecase := cases.Title(language.English)

	input := []byte("\x1b[32mhello,\x1b[0m world!")
	tr := NewANSITransformer(titlecase)
	got, _, err := transform.Bytes(tr, input)
	require.NoError(t, err)
	require.Equal(t, []byte("\x1b[32mHello,\x1b[0m World!"), got)
}

func TestReset(t *testing.T) {
	titlecase := cases.Title(language.English)

	tr := NewANSITransformer(titlecase)

	// Partial ANSI segment
	half := "\x1b[31mfoo"
	_, _, err := transform.Bytes(tr, []byte(half))
	require.NoError(t, err)

	// Reset and transform fresh input
	tr.Reset()
	full := "\x1b[31mbar\x1b[0m"
	out, _, err := transform.Bytes(tr, []byte(full))
	require.NoError(t, err)
	require.Equal(t, []byte("\x1b[31mBar\x1b[0m"), out)
}

func TestIncremental(t *testing.T) {
	type chunkedInput struct {
		name   string
		chunks [][]byte
		want   string
	}
	tests := []chunkedInput{
		{
			name:   "split inside text",
			chunks: [][]byte{[]byte("\x1b[31mhel"), []byte("lo, world!\x1b[0m")},
			want:   "\x1b[31mHello, World!\x1b[0m",
		},
		{
			name:   "split at escape start",
			chunks: [][]byte{[]byte("foo "), []byte("\x1b[31mbar\x1b[0m baz")},
			want:   "Foo \x1b[31mBar\x1b[0m Baz",
		},
		{
			name:   "split inside escape sequence",
			chunks: [][]byte{[]byte("foo \x1b[3"), []byte("1mbar\x1b[0m baz")},
			want:   "Foo \x1b[31mBar\x1b[0m Baz",
		},
		{
			name:   "split after escape sequence",
			chunks: [][]byte{[]byte("foo \x1b[31m"), []byte("bar\x1b[0m baz")},
			want:   "Foo \x1b[31mBar\x1b[0m Baz",
		},
		{
			name:   "split in non-ASCII text",
			chunks: [][]byte{[]byte("héllo "), []byte("\x1b[34mwörld\x1b[0m")},
			want:   "Héllo \x1b[34mWörld\x1b[0m",
		},
		{
			name:   "split incomplete escape at end",
			chunks: [][]byte{[]byte("foo \x1b[3"), []byte("1mbar")},
			want:   "Foo \x1b[31mBar",
		},
		{
			name:   "split with only escape sequence",
			chunks: [][]byte{[]byte("\x1b[31"), []byte("m\x1b[0m")},
			want:   "\x1b[31m\x1b[0m",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			titlecase := cases.Title(language.English)
			tr := NewANSITransformer(titlecase)

			// Allocate a buffer large enough for all output
			out := make([]byte, 0, 128)
			var tmp [128]byte
			atEOF := false

			for i, chunk := range tc.chunks {
				if i == len(tc.chunks)-1 {
					atEOF = true
				}
				n, _, err := tr.Transform(tmp[:], chunk, atEOF)
				require.NoError(t, err)
				out = append(out, tmp[:n]...)
			}
			require.Equal(t, tc.want, string(out))
		})
	}
}

// NopResetter provides Reset
type nonSpanningTransformer struct{ transform.NopResetter }

func (t nonSpanningTransformer) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	// Minimal transform for testing.
	if len(dst) < len(src) {
		copy(dst, src[:len(dst)])
		return len(dst), len(dst), transform.ErrShortDst
	}

	copy(dst, src)
	return len(src), len(src), nil
}

var _ transform.Transformer = nonSpanningTransformer{}

func TestSpanInnerNotSpanning(t *testing.T) {
	tests := []struct {
		name  string
		input string
		atEOF bool
		wantN int
	}{
		{
			name:  "empty",
			input: "",
			atEOF: true,
			wantN: 0,
		},
		{
			name:  "plain text",
			input: "hello",
			atEOF: true,
			wantN: 0,
		},
		{
			name:  "single SGR sequence",
			input: "\x1b[31m",
			atEOF: true,
			wantN: len("\x1b[31m"),
		},
		{
			name:  "multiple SGR sequences",
			input: "\x1b[31m\x1b[1m",
			atEOF: true,
			wantN: len("\x1b[31m\x1b[1m"),
		},
		{
			name:  "SGR then text",
			input: "\x1b[31mhello",
			atEOF: true,
			wantN: len("\x1b[31m"),
		},
		{
			name:  "text then SGR",
			input: "hello\x1b[31m",
			atEOF: true,
			wantN: 0,
		},
		{
			name:  "SGR, text, SGR",
			input: "\x1b[1mhello\x1b[0m",
			atEOF: true,
			wantN: len("\x1b[1m"),
		},
		{
			// "a" is plain text, stops span
			name:  "text, SGR, text",
			input: "a\x1b[1mb\x1b[0mc",
			atEOF: true,
			wantN: 0,
		},
		{
			// "\x1b[1m", then "a" stops span
			name:  "SGR, text, SGR, text",
			input: "\x1b[1ma\x1b[0mb",
			atEOF: true,
			wantN: len("\x1b[1m"),
		},
		{
			name:  "partial SGR sequence",
			input: "\x1b[31",
			atEOF: false,
			wantN: 0,
		},
		{
			name:  "text then partial SGR",
			input: "text\x1b[31",
			atEOF: false,
			wantN: 0,
		},
		{
			name:  "SGR then partial SGR",
			input: "\x1b[32m\x1b[0m\x1b[31",
			atEOF: false,
			wantN: len("\x1b[32m\x1b[0m"),
		},
		{
			name:  "SGR, text, then partial SGR",
			input: "\x1b[32mtext\x1b[0m\x1b[31",
			atEOF: false,
			wantN: len("\x1b[32m"),
		},
		{
			name:  "non-SGR ANSI (treated as text)",
			input: "\x1b[2Jhello",
			atEOF: true,
			wantN: 0,
		},
		{
			name:  "lone ESC (partial)",
			input: "\x1b",
			atEOF: false,
			wantN: 0,
		},
		{
			name:  "ESC then non-bracket (text)",
			input: "\x1bA",
			atEOF: true,
			wantN: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			inner := nonSpanningTransformer{}
			tr := NewANSITransformer(inner)

			n, err := tr.Span([]byte(tc.input), tc.atEOF)
			require.NoError(t, err)
			require.Equal(t, tc.wantN, n, "Input: %q", tc.input)
		})
	}
}

// mockSpanningTransformer is a helper for testing SpanningTransformer behavior.
type mockSpanningTransformer struct {
	// spanFunc will be called by ANSITransformer.Span
	spanFunc func(src []byte, atEOF bool) (n int, err error)

	// transformFunc will be called by ANSITransformer.Transform
	transformFunc func(dst, src []byte, atEOF bool) (nDst, nSrc int, err error)

	// resetCalled tracks if Reset was called
	resetCalled bool
}

func (m *mockSpanningTransformer) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	if m.transformFunc != nil {
		return m.transformFunc(dst, src, atEOF)
	}

	nc := copy(dst, src)
	if nc < len(src) {
		return nc, nc, transform.ErrShortDst
	}

	return nc, nc, nil
}

func (m *mockSpanningTransformer) Span(src []byte, atEOF bool) (n int, err error) {
	if m.spanFunc == nil {
		return 0, fmt.Errorf("mockSpanningTransformer.spanFunc was not set but Span was called")
	}

	return m.spanFunc(src, atEOF)
}

func (m *mockSpanningTransformer) Reset() {
	m.resetCalled = true
}

var _ transform.SpanningTransformer = &mockSpanningTransformer{}

func TestSpanInnerSpanning(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		atEOF       bool
		mockSpanMap map[string]int
		wantN       int
	}{
		{name: "empty", input: "", atEOF: true, mockSpanMap: map[string]int{}, wantN: 0},
		{
			name:        "plain text, inner spans all",
			input:       "hello",
			atEOF:       true,
			mockSpanMap: map[string]int{"hello": len("hello")},
			wantN:       len("hello"),
		},
		{
			name:        "plain text, inner spans none",
			input:       "hello",
			atEOF:       true,
			mockSpanMap: map[string]int{"hello": 0},
			wantN:       0,
		},
		{
			// Span stops before "helloworld" as inner would transform part of it
			name:        "plain text, inner spans some (triggers transform)",
			input:       "helloworld",
			atEOF:       true,
			mockSpanMap: map[string]int{"helloworld": 5},
			wantN:       0,
		},
		{
			// No plain text segments for inner to span
			name:        "SGR sequence only",
			input:       "\x1b[31m\x1b[0m",
			atEOF:       true,
			mockSpanMap: map[string]int{},
			wantN:       len("\x1b[31m\x1b[0m"),
		},
		{
			// Segments: "\x1b[31m", "TEXT", "\x1b[0m", "hello"
			name:        "SGR then text, inner spans text",
			input:       "\x1b[31mTEXT\x1b[0mhello",
			atEOF:       true,
			mockSpanMap: map[string]int{"TEXT": len("TEXT"), "hello": len("hello")},
			wantN:       len("\x1b[31mTEXT\x1b[0mhello"),
		},
		{
			// Segments: "\x1b[31m", "TEXT", "\x1b[0m", "transform"
			name:        "SGR then text, inner does not span latter text",
			input:       "\x1b[31mTEXT\x1b[0mtransform",
			atEOF:       true,
			mockSpanMap: map[string]int{"TEXT": len("TEXT"), "transform": 0},
			// Span stops before "transform"
			wantN: len("\x1b[31mTEXT\x1b[0m"),
		},
		{
			// Segments: "hello", "\x1b[31m", "TEXT", "\x1b[0m"
			name:        "text then SGR, inner spans text",
			input:       "hello\x1b[31mTEXT\x1b[0m",
			atEOF:       true,
			mockSpanMap: map[string]int{"hello": len("hello"), "TEXT": len("TEXT")},
			wantN:       len("hello\x1b[31mTEXT\x1b[0m"),
		},
		{
			name:        "text then SGR, inner does not span text",
			input:       "hello\x1b[31mTEXT\x1b[0m",
			atEOF:       true,
			mockSpanMap: map[string]int{"hello": 0, "TEXT": len("TEXT")},
			// Span stops before "hello"
			wantN: 0,
		},
		{
			// Segments: "text1", "\x1b[1m", "BOLD", "\x1b[0m", "text2"
			name:        "interleaved, all text spanned by inner",
			input:       "text1\x1b[1mBOLD\x1b[0mtext2",
			atEOF:       true,
			mockSpanMap: map[string]int{"text1": len("text1"), "BOLD": len("BOLD"), "text2": len("text2")},
			wantN:       len("text1\x1b[1mBOLD\x1b[0mtext2"),
		},
		{
			name:        "interleaved, middle text not spanned by inner",
			input:       "text1\x1b[1mtransform_me\x1b[0mtext2",
			atEOF:       true,
			mockSpanMap: map[string]int{"text1": len("text1"), "transform_me": 0, "text2": len("text2")},
			// Span stops before "transform_me"
			wantN: len("text1\x1b[1m"),
		},
		{
			// Segments: "text1", "\x1b[31" (Partial)
			name:        "text, then partial SGR, inner spans text",
			input:       "text1\x1b[31",
			atEOF:       false,
			mockSpanMap: map[string]int{"text1": len("text1")},
			// Span stops before partial sequence
			wantN: len("text1"),
		},
		{
			name: "non-SGR ANSI (text), inner spans all",
			// Whole thing is one PlainText segment, as the SGR sequence is invalid
			input:       "\x1b[2Jhello",
			atEOF:       true,
			mockSpanMap: map[string]int{"\x1b[2Jhello": len("\x1b[2Jhello")},
			wantN:       len("\x1b[2Jhello"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mockInner := &mockSpanningTransformer{
				spanFunc: func(src []byte, atEOF bool) (n int, err error) {
					sSrc := string(src)
					if spanVal, ok := tc.mockSpanMap[sSrc]; ok {
						return spanVal, nil
					}

					// If a plain text segment is encountered by ANSITransformer that's not in mockSpanMap,
					// assume for this test setup that the inner transformer would want to transform it.
					t.Logf("mockSpanningTransformer.spanFunc received unexpected plain text segment: %q (defaulting to span 0)", sSrc)
					return 0, nil
				},
			}

			tr := NewANSITransformer(mockInner)
			n, err := tr.Span([]byte(tc.input), tc.atEOF)
			require.NoError(t, err)
			require.Equal(t, tc.wantN, n, "Input: %q", tc.input)
		})
	}
}

// setupTransformerWithLeftover uses a real SpanningTransformer (cases.Title) to
// populate the leftover buffer of an ANSITransformer. It returns the
// transformer instance and asserts that leftover was created.
func setupTransformerWithLeftover(t *testing.T) *ANSITransformer {
	t.Helper()

	titleCaser := cases.Title(language.English)
	ansiTransformer := NewANSITransformer(titleCaser)
	//
	// "text" will be transformed, "\x1b[31" becomes leftover (this isn't a
	// complete SGR sequence - no `m`)
	inputWithPartial := []byte("text\x1b[31")
	destBuf := make([]byte, 100)
	_, nSrcConsumed, err := ansiTransformer.Transform(destBuf, inputWithPartial, false)

	require.NoError(t, err, "Transform call to create leftover state failed")
	require.Equal(t, len("text"), nSrcConsumed, "Transform should consume text before partial")
	require.NotEmpty(t, ansiTransformer.leftover, "Leftover buffer should be populated")
	require.Equal(t, []byte("\x1b[31"), ansiTransformer.leftover, "Leftover content mismatch")

	return ansiTransformer
}

func TestSpanWithExistingLeftover(t *testing.T) {
	t.Parallel()

	ansiTransformerWithState := setupTransformerWithLeftover(t)

	// Call Span on the transformer instance (which now has leftover bytes).
	// `ANSITransformer.Span` should return `0` immediately because of the
	// leftover bytes. The inner transformer's `Span` method should not be called
	// in this case.
	spanN, spanErr := ansiTransformerWithState.Span([]byte("more data to span"), false)
	require.NoError(t, spanErr, "Span with existing leftover should return (0, nil)")
	require.Equal(t, 0, spanN, "Span should return 0 if leftover bytes exist")
}

func TestSpanAfterResetWithRealSpanningInner(t *testing.T) {
	ansiTransformer := setupTransformerWithLeftover(t)

	ansiTransformer.Reset()
	require.Empty(t, ansiTransformer.leftover, "Leftover should be cleared after Reset")

	// Now `Span` should operate on "more data to span" without being affected by
	// prior leftover. `ansiTransformer` wraps `titleCaser` (which is a
	// `SpanningTransformer`). `titleCaser.Span("more data to span", false)` will
	// return `(0, transform.ErrEndOfSpan)` because "more data to span" would be
	// transformed to "More Data To Span". `ANSITransformer.Span` should correctly
	// interpret this: it cannot span the text, so it should return `(0, nil)`
	// because the span stops before this text.
	spanNAfterReset, spanErrAfterReset := ansiTransformer.Span([]byte("more data to span"), false)
	require.NoError(t, spanErrAfterReset, "Span after reset should correctly handle inner Span's ErrEndOfSpan and return nil error")
	require.Equal(t, 0, spanNAfterReset, "Span should return 0 because inner (titleCaser) needs to transform 'more data to span'")
}

func TestSpanAfterResetWithMockSpanningInner(t *testing.T) {
	mockInner := &mockSpanningTransformer{
		spanFunc: func(src []byte, atEOF bool) (n int, err error) {
			// Mock inner transformer spans all text without transforming
			return len(src), nil
		},
	}

	mockTransformer := NewANSITransformer(mockInner)

	// Simulate leftover state then reset it
	mockTransformer.leftover = []byte("simulated leftover")
	mockTransformer.Reset()

	require.True(t, mockInner.resetCalled, "Inner transformer's Reset should be called")
	require.Empty(t, mockTransformer.leftover, "Leftover should be cleared after Reset")

	// `Span` should now proceed normally. Since `mockInner.spanFunc` returns
	// `(len(src), nil)`, `ANSITransformer.Span` should also span the entire
	// input.
	spanN, err := mockTransformer.Span([]byte("fully spannable data"), false)
	require.NoError(t, err)
	require.Equal(t, len("fully spannable data"), spanN, "Span should proceed normally and span all data with a fully spanning mock inner after Reset")
}
