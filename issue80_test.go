package imap

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"
)

// simulateExecReadLine reproduces the literal-reading loop from Exec (exec.go:46-72)
func simulateExecReadLine(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadBytes('\n')
	if err != nil && err != io.EOF {
		return line, err
	}
	for {
		if a := atom.Find(dropNl(line)); a != nil {
			sizeStr := string(a[1 : len(a)-1])
			sizeStr = strings.TrimSuffix(sizeStr, "+")
			n, atoiErr := strconv.Atoi(sizeStr)
			if atoiErr != nil {
				return nil, atoiErr
			}
			buf := make([]byte, n)
			_, readErr := io.ReadFull(r, buf)
			if readErr != nil {
				return nil, readErr
			}
			line = append(line, buf...)
			buf, err = r.ReadBytes('\n')
			if err != nil && err != io.EOF {
				return nil, err
			}
			line = append(line, buf...)
			continue
		}
		break
	}
	return line, nil
}

// simulateExecBuildResponse reads all lines until tag, building response string
func simulateExecBuildResponse(wireData string, tag string) (string, error) {
	r := bufio.NewReader(strings.NewReader(wireData))
	var resp strings.Builder
	for {
		line, err := simulateExecReadLine(r)
		if err != nil {
			return "", fmt.Errorf("simulateExecReadLine: %w", err)
		}
		if len(line) >= len(tag)+3 && string(line[:len(tag)]) == tag {
			break
		}
		resp.Write(line)
	}
	return resp.String(), nil
}

func TestIssue80_LiteralPlusExecAssembly(t *testing.T) {
	// RFC 7888 LITERAL+ uses {NNN+}\r\n syntax. Gmail advertises LITERAL+.
	// When the server uses this syntax in responses, Exec must read the
	// literal body as a block (just like standard {NNN}\r\n).

	body := "Subject: Welcome to YouTube\r\n\r\n<html>(content)</html>\r\n"
	size := len(body)

	wire := fmt.Sprintf(
		"* 7 FETCH (UID 7 BODY[] {%d+}\r\n%s)\r\nTAG00000000000000 OK done\r\n",
		size, body,
	)

	resp, err := simulateExecBuildResponse(wire, "TAG00000000000000")
	if err != nil {
		t.Fatalf("simulateExecBuildResponse error: %v", err)
	}

	expected := fmt.Sprintf("* 7 FETCH (UID 7 BODY[] {%d+}\r\n%s)\r\n", size, body)
	if resp != expected {
		t.Errorf("Exec assembled response incorrectly with LITERAL+.\ngot:  %q\nwant: %q", resp, expected)
	}
}

func TestIssue80_LiteralPlusParseFetchTokens(t *testing.T) {
	// parseFetchTokens must handle {NNN+}\r\n the same as {NNN}\r\n.
	// The '+' indicates non-synchronizing literal (RFC 7888) and should
	// be skipped when parsing the literal size.

	body := "Hello World (with parens)\r\n"
	size := len(body)

	input := fmt.Sprintf("(UID 7 BODY[] {%d+}\r\n%s)", size, body)
	tokens, err := parseFetchTokens(input)
	if err != nil {
		t.Fatalf("parseFetchTokens with LITERAL+ failed: %v", err)
	}
	if len(tokens) != 4 {
		t.Fatalf("expected 4 tokens, got %d", len(tokens))
	}
	if tokens[0].Type != TLiteral || tokens[0].Str != "UID" {
		t.Errorf("token 0: expected UID literal, got %+v", tokens[0])
	}
	if tokens[1].Type != TNumber || tokens[1].Num != 7 {
		t.Errorf("token 1: expected 7, got %+v", tokens[1])
	}
	if tokens[2].Type != TLiteral || tokens[2].Str != "BODY[]" {
		t.Errorf("token 2: expected BODY[], got %+v", tokens[2])
	}
	if tokens[3].Type != TAtom || tokens[3].Str != body {
		t.Errorf("token 3: expected body atom, got type=%s str=%q", GetTokenName(tokens[3].Type), tokens[3].Str)
	}
}

func TestIssue80_LiteralPlusFindFetchContentEnd(t *testing.T) {
	// findFetchContentEnd must recognize {NNN+}\r\n as a literal.
	// Without this, body content containing () corrupts depth tracking.

	body := "<html>(content with parens) and (more(nested))</html>\r\n"
	size := len(body)

	resp := fmt.Sprintf("* 7 FETCH (UID 7 BODY[] {%d+}\r\n%s)\r\n", size, body)
	d := &Dialer{}
	recs, err := d.ParseFetchResponse(resp)
	if err != nil {
		t.Fatalf("ParseFetchResponse with LITERAL+ failed: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	if len(recs[0]) != 4 {
		t.Fatalf("expected 4 tokens, got %d", len(recs[0]))
	}
	if recs[0][3].Type != TAtom || recs[0][3].Str != body {
		t.Errorf("body mismatch: type=%s str=%q", GetTokenName(recs[0][3].Type), recs[0][3].Str)
	}
}

func TestIssue80_LiteralPlusMultiRecord(t *testing.T) {
	// Two FETCH records with LITERAL+ syntax.
	body1 := "Subject: Welcome to YouTube\r\n\r\n<html>(video link)</html>\r\n"
	body2 := "Subject: Other\r\n\r\nPlain text\r\n"

	resp := fmt.Sprintf(
		"* 7 FETCH (UID 7 BODY[] {%d+}\r\n%s)\r\n"+
			"* 8 FETCH (UID 8 BODY[] {%d+}\r\n%s)\r\n",
		len(body1), body1, len(body2), body2,
	)

	d := &Dialer{}
	recs, err := d.ParseFetchResponse(resp)
	if err != nil {
		t.Fatalf("ParseFetchResponse error: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 records, got %d", len(recs))
	}
	if recs[0][3].Str != body1 {
		t.Errorf("body1 mismatch: got %q", recs[0][3].Str)
	}
	if recs[1][3].Str != body2 {
		t.Errorf("body2 mismatch: got %q", recs[1][3].Str)
	}
}

func TestIssue80_LiteralPlusEmptyBody(t *testing.T) {
	// LITERAL+ with empty body {0+}
	resp := "* 7 FETCH (UID 7 BODY[] {0+}\r\n)\r\n"
	d := &Dialer{}
	recs, err := d.ParseFetchResponse(resp)
	if err != nil {
		t.Fatalf("ParseFetchResponse error: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
}

func TestIssue80_LiteralPlusMixedWithStandard(t *testing.T) {
	// Mix of standard and LITERAL+ literals in the same response
	body1 := "Body one (with parens)\r\n"
	body2 := "Body two (also parens)\r\n"

	resp := fmt.Sprintf(
		"* 7 FETCH (UID 7 BODY[] {%d}\r\n%s)\r\n"+
			"* 8 FETCH (UID 8 BODY[] {%d+}\r\n%s)\r\n",
		len(body1), body1, len(body2), body2,
	)

	d := &Dialer{}
	recs, err := d.ParseFetchResponse(resp)
	if err != nil {
		t.Fatalf("ParseFetchResponse error: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 records, got %d", len(recs))
	}
	if recs[0][3].Str != body1 {
		t.Errorf("body1 mismatch: got %q", recs[0][3].Str)
	}
	if recs[1][3].Str != body2 {
		t.Errorf("body2 mismatch: got %q", recs[1][3].Str)
	}
}
