package imap

import (
	"fmt"
	"strings"
	"testing"
)

// --- parseFetchTokens coverage ---

func TestParseFetchTokens_QuotedStrings(t *testing.T) {
	tokens, err := parseFetchTokens(`(SUBJECT "hello world")`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(tokens))
	}
	if tokens[1].Type != TQuoted || tokens[1].Str != "hello world" {
		t.Errorf("expected quoted 'hello world', got %+v", tokens[1])
	}
}

func TestParseFetchTokens_QuotedWithEscapes(t *testing.T) {
	tokens, err := parseFetchTokens(`(SUBJECT "say \"hi\"")`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(tokens))
	}
	if tokens[1].Type != TQuoted || tokens[1].Str != `say "hi"` {
		t.Errorf("expected quoted with unescaped quotes, got %+v", tokens[1])
	}
}

func TestParseFetchTokens_EmptyQuoted(t *testing.T) {
	tokens, err := parseFetchTokens(`(SUBJECT "")`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(tokens))
	}
	if tokens[1].Type != TQuoted || tokens[1].Str != "" {
		t.Errorf("expected empty quoted, got %+v", tokens[1])
	}
}

func TestParseFetchTokens_NIL(t *testing.T) {
	tokens, err := parseFetchTokens(`(SUBJECT NIL)`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(tokens))
	}
	if tokens[1].Type != TNil {
		t.Errorf("expected TNil, got %+v", tokens[1])
	}
}

func TestParseFetchTokens_UnmatchedCloseParen(t *testing.T) {
	_, err := parseFetchTokens(`)`)
	if err == nil {
		t.Fatal("expected error for unmatched ')'")
	}
	if !strings.Contains(err.Error(), "unmatched ')'") {
		t.Errorf("expected unmatched ')' error, got: %v", err)
	}
}

func TestParseFetchTokens_MismatchedParens(t *testing.T) {
	_, err := parseFetchTokens(`((FOO)`)
	if err == nil {
		t.Fatal("expected error for mismatched parentheses")
	}
	if !strings.Contains(err.Error(), "mismatched parentheses") {
		t.Errorf("expected mismatched parentheses error, got: %v", err)
	}
}

func TestParseFetchTokens_LiteralPlusInvalidAfterPlus(t *testing.T) {
	// {3+x should error — '+' not followed by '}'
	_, err := parseFetchTokens(`(BODY {3+x)`)
	if err == nil {
		t.Fatal("expected error for invalid char after '+' in literal")
	}
	if !strings.Contains(err.Error(), "expected '}'") {
		t.Errorf("expected '}' after '+' error, got: %v", err)
	}
}

func TestParseFetchTokens_LiteralPlusTruncatedAfterPlus(t *testing.T) {
	// {3+ at end of input — '+' with nothing after
	_, err := parseFetchTokens(`(BODY {3+`)
	if err == nil {
		t.Fatal("expected error for truncated literal after '+'")
	}
	if !strings.Contains(err.Error(), "expected '}'") {
		t.Errorf("expected '}' after '+' error, got: %v", err)
	}
}

func TestParseFetchTokens_NestedContainers(t *testing.T) {
	tokens, err := parseFetchTokens(`(FLAGS (\\Seen (\\Draft)))`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(tokens))
	}
	if tokens[1].Type != TContainer {
		t.Errorf("expected container, got %+v", tokens[1])
	}
}

func TestParseFetchTokens_DeepNesting(t *testing.T) {
	// Force container stack growth (depth > 4, initial capacity)
	tokens, err := parseFetchTokens(`(A (B (C (D (E F)))))`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(tokens))
	}
}

func TestParseFetchTokens_TrailingLiteral(t *testing.T) {
	// Literal at end of input without container — triggers end-of-input pushToken
	tokens, err := parseFetchTokens(`FOO`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(tokens))
	}
	if tokens[0].Type != TLiteral || tokens[0].Str != "FOO" {
		t.Errorf("expected FOO literal, got %+v", tokens[0])
	}
}

func TestParseFetchTokens_NumberToken(t *testing.T) {
	tokens, err := parseFetchTokens(`(UID 42)`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(tokens))
	}
	if tokens[1].Type != TNumber || tokens[1].Num != 42 {
		t.Errorf("expected number 42, got %+v", tokens[1])
	}
}

func TestParseFetchTokens_SingleContainerUnwrap(t *testing.T) {
	// When the entire input is a single container, tokens are unwrapped
	tokens, err := parseFetchTokens(`(UID 7)`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should unwrap — tokens should be UID and 7, not a single container
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens after unwrap, got %d", len(tokens))
	}
}

// --- findFetchContentEnd coverage ---

func TestFindFetchContentEnd_EmptyInput(t *testing.T) {
	end, err := findFetchContentEnd("", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if end != 0 {
		t.Errorf("expected 0, got %d", end)
	}
}

func TestFindFetchContentEnd_PastEnd(t *testing.T) {
	end, err := findFetchContentEnd("abc", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if end != 3 {
		t.Errorf("expected 3, got %d", end)
	}
}

func TestFindFetchContentEnd_WhitespaceOnly(t *testing.T) {
	end, err := findFetchContentEnd("   ", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if end != 3 {
		t.Errorf("expected 3, got %d", end)
	}
}

func TestFindFetchContentEnd_NoParen(t *testing.T) {
	// Non-paren content falls back to findLineEnd
	s := "SOME DATA\r\nNEXT"
	end, err := findFetchContentEnd(s, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if end != 9 { // index of \r
		t.Errorf("expected 9, got %d", end)
	}
}

func TestFindFetchContentEnd_NoParenNoNewline(t *testing.T) {
	s := "SOME DATA"
	end, err := findFetchContentEnd(s, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if end != len(s) {
		t.Errorf("expected %d, got %d", len(s), end)
	}
}

func TestFindFetchContentEnd_QuotedWithEscape(t *testing.T) {
	// Quoted string containing escaped quote and backslash
	s := `(SUBJECT "say \"hi\"")`
	end, err := findFetchContentEnd(s, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if end != len(s) {
		t.Errorf("expected %d, got %d", len(s), end)
	}
}

func TestFindFetchContentEnd_Unterminated(t *testing.T) {
	_, err := findFetchContentEnd("(FOO BAR", 0)
	if err == nil {
		t.Fatal("expected error for unterminated FETCH response")
	}
	if !strings.Contains(err.Error(), "unterminated") {
		t.Errorf("expected unterminated error, got: %v", err)
	}
}

func TestFindFetchContentEnd_LiteralExceedsBuffer(t *testing.T) {
	// Literal declares more bytes than available
	_, err := findFetchContentEnd("({999}\r\nshort)", 0)
	if err == nil {
		t.Fatal("expected error for literal size exceeding buffer")
	}
	if !strings.Contains(err.Error(), "exceeds remaining buffer") {
		t.Errorf("expected buffer overflow error, got: %v", err)
	}
}

func TestFindFetchContentEnd_LiteralSizeOverflow(t *testing.T) {
	// Integer overflow makes Atoi fail even though the loop only collects digits
	_, err := findFetchContentEnd("({99999999999999999999}\r\ndata)", 0)
	if err == nil {
		t.Fatal("expected error for overflowing literal size")
	}
	if !strings.Contains(err.Error(), "parse literal size") {
		t.Errorf("expected parse literal size error, got: %v", err)
	}
}

func TestFindFetchContentEnd_BraceNotLiteral(t *testing.T) {
	// { not followed by digits+} — should be treated as regular char
	end, err := findFetchContentEnd("({nodigits})", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if end != 12 {
		t.Errorf("expected 12, got %d", end)
	}
}

// --- ParseFetchResponse coverage ---

func TestParseFetchResponse_EmptyInput(t *testing.T) {
	d := &Dialer{}
	recs, err := d.ParseFetchResponse("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recs) != 0 {
		t.Errorf("expected 0 records, got %d", len(recs))
	}
}

func TestParseFetchResponse_WhitespaceOnly(t *testing.T) {
	d := &Dialer{}
	recs, err := d.ParseFetchResponse("   \r\n  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recs) != 0 {
		t.Errorf("expected 0 records, got %d", len(recs))
	}
}

func TestParseFetchResponse_NoFetchNoStar(t *testing.T) {
	d := &Dialer{}
	recs, err := d.ParseFetchResponse("just some random text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recs) != 0 {
		t.Errorf("expected 0 records, got %d", len(recs))
	}
}

func TestParseFetchResponse_QuotedStringInFetch(t *testing.T) {
	d := &Dialer{}
	resp := "* 1 FETCH (SUBJECT \"hello world\" UID 5)\r\n"
	recs, err := d.ParseFetchResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	// SUBJECT, "hello world", UID, 5
	if len(recs[0]) != 4 {
		t.Fatalf("expected 4 tokens, got %d", len(recs[0]))
	}
	if recs[0][1].Type != TQuoted || recs[0][1].Str != "hello world" {
		t.Errorf("expected quoted 'hello world', got %+v", recs[0][1])
	}
}

func TestParseFetchResponse_LiteralWithCRLFOnly(t *testing.T) {
	// Literal with \r\n after size but before data
	body := "test"
	resp := fmt.Sprintf("* 1 FETCH (BODY {%d}\r\n%s)\r\n", len(body), body)
	d := &Dialer{}
	recs, err := d.ParseFetchResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	if recs[0][1].Type != TAtom || recs[0][1].Str != body {
		t.Errorf("expected body atom %q, got %+v", body, recs[0][1])
	}
}

// --- GetTokenName coverage ---

func TestGetTokenName(t *testing.T) {
	tests := []struct {
		tt   TType
		want string
	}{
		{TUnset, "TUnset"},
		{TAtom, "TAtom"},
		{TNumber, "TNumber"},
		{TLiteral, "TLiteral"},
		{TQuoted, "TQuoted"},
		{TNil, "TNil"},
		{TContainer, "TContainer"},
		{TType(99), ""},
	}
	for _, tc := range tests {
		got := GetTokenName(tc.tt)
		if got != tc.want {
			t.Errorf("GetTokenName(%d) = %q, want %q", tc.tt, got, tc.want)
		}
	}
}

// --- Token.String() coverage ---

func TestTokenString(t *testing.T) {
	tests := []struct {
		name  string
		token Token
		want  string
	}{
		{"unset", Token{Type: TUnset}, "TUnset"},
		{"nil", Token{Type: TNil}, "TNil"},
		{"number", Token{Type: TNumber, Num: 42}, "(TNumber 42)"},
		{"literal", Token{Type: TLiteral, Str: "BODY"}, "(TLiteral BODY)"},
		{"atom", Token{Type: TAtom, Str: "hello"}, fmt.Sprintf("(TAtom, len 5, chars 5 %#v)", "hello")},
		{"quoted", Token{Type: TQuoted, Str: "hi"}, fmt.Sprintf("(TQuoted, len 2, chars 2 %#v)", "hi")},
		{"container", Token{Type: TContainer, Tokens: []*Token{{Type: TLiteral, Str: "A"}}}, "(TContainer children: [(TLiteral A)])"},
		{"unknown", Token{Type: TType(99)}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.token.String()
			if got != tc.want {
				t.Errorf("Token.String() = %q, want %q", got, tc.want)
			}
		})
	}
}

// --- CheckType coverage ---

func TestCheckType(t *testing.T) {
	d := &Dialer{ConnNum: 1, Folder: "INBOX"}

	t.Run("accepted type", func(t *testing.T) {
		token := &Token{Type: TLiteral, Str: "UID"}
		err := d.CheckType(token, []TType{TLiteral, TNumber}, nil, "test %d", 1)
		if err != nil {
			t.Errorf("expected nil error, got: %v", err)
		}
	})

	t.Run("rejected type", func(t *testing.T) {
		token := &Token{Type: TQuoted, Str: "bad"}
		err := d.CheckType(token, []TType{TLiteral, TNumber}, []*Token{token}, "test %d", 1)
		if err == nil {
			t.Fatal("expected error for rejected type")
		}
		if !strings.Contains(err.Error(), "TLiteral|TNumber") {
			t.Errorf("expected acceptable types in error, got: %v", err)
		}
		if !strings.Contains(err.Error(), "IMAP1:INBOX") {
			t.Errorf("expected IMAP conn info in error, got: %v", err)
		}
	})
}

// --- findLineEnd coverage ---

func TestFindLineEnd(t *testing.T) {
	t.Run("with CRLF", func(t *testing.T) {
		s := "hello\r\nworld"
		end := findLineEnd(s, 0)
		if end != 5 {
			t.Errorf("expected 5, got %d", end)
		}
	})

	t.Run("no CRLF", func(t *testing.T) {
		s := "hello world"
		end := findLineEnd(s, 0)
		if end != len(s) {
			t.Errorf("expected %d, got %d", len(s), end)
		}
	})

	t.Run("from offset", func(t *testing.T) {
		s := "abc\r\ndef\r\nghi"
		end := findLineEnd(s, 5)
		if end != 8 {
			t.Errorf("expected 8, got %d", end)
		}
	})
}
