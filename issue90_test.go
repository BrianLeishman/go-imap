package imap

import (
	"strings"
	"testing"
)

// Fastmail (and other servers) return custom keyword flags such as
// $X-ME-Annot-2 and $CanUnsubscribe. These contain '$' and '-', which
// are valid atom-chars per RFC 3501 but were previously dropped by the
// tokenizer, causing "$X-ME-Annot-2" to be split into ["X", "ME",
// "Annot", 2] and FLAGS parsing to fail with:
//   expected TLiteral token for FLAGS[3], got (TNumber 2)

func TestIssue90_FastmailKeywordFlagsTokenize(t *testing.T) {
	tokens, err := parseFetchTokens(`(FLAGS ($X-ME-Annot-2 $CanUnsubscribe \Seen))`)
	if err != nil {
		t.Fatalf("parseFetchTokens: %v", err)
	}
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens (FLAGS, container), got %d: %v", len(tokens), tokens)
	}
	if tokens[0].Type != TLiteral || tokens[0].Str != "FLAGS" {
		t.Fatalf("token 0: expected TLiteral FLAGS, got %+v", tokens[0])
	}
	if tokens[1].Type != TContainer {
		t.Fatalf("token 1: expected TContainer, got %+v", tokens[1])
	}
	got := tokens[1].Tokens
	want := []string{"$X-ME-Annot-2", "$CanUnsubscribe", `\Seen`}
	if len(got) != len(want) {
		t.Fatalf("expected %d flag tokens, got %d: %v", len(want), len(got), got)
	}
	for i, w := range want {
		if got[i].Type != TLiteral {
			t.Errorf("flag[%d]: expected TLiteral, got %s (%q)", i, GetTokenName(got[i].Type), got[i].Str)
			continue
		}
		if got[i].Str != w {
			t.Errorf("flag[%d]: expected %q, got %q", i, w, got[i].Str)
		}
	}
}

func TestIssue90_FastmailOverviewParse(t *testing.T) {
	// Real-world-ish FETCH response from Fastmail with custom keyword flags
	// mixed in. Regression test for the full ParseFetchResponse pipeline.
	resp := "* 1 FETCH (FLAGS ($X-ME-Annot-2 $CanUnsubscribe) " +
		"UID 18467 " +
		"INTERNALDATE \" 9-Apr-2026 17:06:19 -0400\" " +
		"RFC822.SIZE 49862 " +
		"ENVELOPE (\"Thu, 9 Apr 2026 21:06:17 +0000\" " +
		"\"AT&T welcomes Quantum Fiber to the family\" " +
		"((\"=?UTF-8?B?QVQmVA==?=\" NIL \"ATT\" \"message.att-mail.com\")) " +
		"((\"=?UTF-8?B?QVQmVA==?=\" NIL \"ATT\" \"message.att-mail.com\")) " +
		"((\"=?UTF-8?B?QVQmVA==?=\" NIL \"reply\" \"message.att-mail.com\")) " +
		"((NIL NIL \"rohit\" \"centurylink.kumbhar.net\")) " +
		"NIL NIL NIL " +
		"\"<0.2.C.BD.1DCC864B50CA79A.0@omp.message.att-mail.com>\"))\r\n"

	d := &Dialer{Folder: "INBOX"}
	records, err := d.ParseFetchResponse(resp)
	if err != nil {
		t.Fatalf("ParseFetchResponse: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	e, err := d.parseOverviewRecord(records[0])
	if err != nil {
		t.Fatalf("parseOverviewRecord: %v", err)
	}
	if e.UID != 18467 {
		t.Errorf("UID: expected 18467, got %d", e.UID)
	}
	wantFlags := map[string]bool{"$X-ME-Annot-2": true, "$CanUnsubscribe": true}
	if len(e.Flags) != len(wantFlags) {
		t.Fatalf("flags: expected %v, got %v", wantFlags, e.Flags)
	}
	for _, f := range e.Flags {
		if !wantFlags[f] {
			t.Errorf("unexpected flag %q (have %v)", f, e.Flags)
		}
	}
}

func TestIssue90_IsLiteralAtomChars(t *testing.T) {
	// RFC 3501 ATOM-CHAR: any CHAR except atom-specials.
	// atom-specials = "(" / ")" / "{" / SP / CTL / list-wildcards
	//                 / quoted-specials / resp-specials
	// list-wildcards  = "%" / "*"
	// quoted-specials = DQUOTE / "\"
	// resp-specials   = "]"
	//
	// This codebase intentionally keeps '\' and ']' accepted for flag
	// ("\Seen") and BODY[...] syntax, but all other atom-specials must
	// be rejected. Everything else (including '}', '$', '-', '+', '_',
	// '#', '/', '@', etc.) is a valid atom-char.
	accept := []rune{
		'$', '-', '_', '+', '}', '#', '/', '@', '!', '&', '\'',
		',', '.', ':', ';', '<', '=', '>', '?', '^', '`', '|', '~',
		'0', '9', 'a', 'Z', '\\', '[', ']',
	}
	for _, c := range accept {
		if !IsLiteral(c) {
			t.Errorf("IsLiteral(%q) = false, want true (valid atom-char)", c)
		}
	}
	reject := []rune{'(', ')', '{', ' ', '%', '*', '"', '\t', '\r', '\n', 0x00, 0x7f}
	for _, c := range reject {
		if IsLiteral(c) {
			t.Errorf("IsLiteral(%q) = true, want false (atom-special or CTL)", c)
		}
	}
}

// Sanity check: make sure the expanded IsLiteral doesn't break parsing
// of \Seen-style system flags embedded in a FLAGS list.
func TestIssue90_SystemFlagsStillWork(t *testing.T) {
	tokens, err := parseFetchTokens(`(FLAGS (\Seen \Answered $Forwarded))`)
	if err != nil {
		t.Fatalf("parseFetchTokens: %v", err)
	}
	if len(tokens) != 2 || tokens[1].Type != TContainer {
		t.Fatalf("unexpected tokens: %v", tokens)
	}
	flags := tokens[1].Tokens
	want := []string{`\Seen`, `\Answered`, `$Forwarded`}
	if len(flags) != len(want) {
		t.Fatalf("expected %d flags, got %d: %v", len(want), len(flags), flags)
	}
	for i, w := range want {
		if flags[i].Type != TLiteral || flags[i].Str != w {
			t.Errorf("flag[%d]: want TLiteral %q, got %s %q", i, w, GetTokenName(flags[i].Type), flags[i].Str)
		}
	}
}

// parseAtomLiteral must reject malformed literal headers. Previously,
// any non-digit byte after the size was silently treated as '}', so
// inputs like "{5Xabcde" parsed a 5-byte literal starting after 'X'
// instead of erroring. Found by GPT-5.4 during code review.
func TestIssue90_MalformedLiteralHeaderRejected(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"garbage char after size", "(BODY {5Xabcde)"},
		{"space after size", "(BODY {5 abcde)"},
		{"close paren after size", "(BODY {5)abcde)"},
		{"letter after size", "(BODY {3a123)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseFetchTokens(tc.input)
			if err == nil {
				t.Fatalf("expected error for malformed literal header %q, got nil", tc.input)
			}
			if !strings.Contains(err.Error(), "literal") && !strings.Contains(err.Error(), "'}'") {
				t.Errorf("expected literal-header error, got: %v", err)
			}
		})
	}
}

// Sanity: valid literal headers still parse after the malformed-header
// rejection is tightened. Covers both standard and LITERAL+ syntax.
func TestIssue90_ValidLiteralHeadersStillParse(t *testing.T) {
	cases := []string{
		"(BODY {5}\r\nhello)",
		"(BODY {5+}\r\nhello)",
		"(BODY {0}\r\n)",
		"(BODY {0+}\r\n)",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			if _, err := parseFetchTokens(in); err != nil {
				t.Errorf("parseFetchTokens(%q) unexpected error: %v", in, err)
			}
		})
	}
}

func TestIssue90_ErrorMessageRegressionGone(t *testing.T) {
	// Reproduces the literal error from issue #90 and asserts we no
	// longer produce it.
	resp := "* 1 FETCH (FLAGS ($X-ME-Annot-2 $CanUnsubscribe) UID 1)\r\n"
	d := &Dialer{Folder: "INBOX"}
	records, err := d.ParseFetchResponse(resp)
	if err != nil {
		t.Fatalf("ParseFetchResponse: %v", err)
	}
	if _, err := d.parseOverviewRecord(records[0]); err != nil {
		if strings.Contains(err.Error(), "expected TLiteral token for FLAGS") {
			t.Fatalf("regression: got old error: %v", err)
		}
		t.Fatalf("unexpected error: %v", err)
	}
}
