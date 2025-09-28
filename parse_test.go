package imap

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseFetchTokensLiteralBoundary(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantErr      bool
		errContains  string
		description  string
		wantTokens   int  // Expected number of tokens
		checkContent bool // Whether to check token content
	}{
		{
			name:         "empty literal {0}",
			input:        "(BODY {0}\r\n)",
			wantErr:      false,
			description:  "Should handle empty literal {0} correctly",
			wantTokens:   2, // BODY and empty atom
			checkContent: true,
		},
		{
			name:         "literal with exact size",
			input:        "(BODY {5}\r\nHello)",
			wantErr:      false,
			description:  "Should handle literal with exact matching size",
			wantTokens:   2, // BODY and "Hello"
			checkContent: true,
		},
		{
			name:        "literal size exceeds buffer - should take available data",
			input:       "(BODY {10}\r\nHello     )",
			wantErr:     false,
			description: "Should handle literal where declared size exceeds available data",
			wantTokens:  2, // BODY and truncated content
		},
		{
			name:        "literal at end with size but no data",
			input:       "(BODY {5}\r\n",
			wantErr:     true,
			errContains: "literal size 5 but tokenStart",
			description: "Should error when literal declares size but has no data",
		},
		{
			name:         "literal with multiline content",
			input:        "(BODY {15}\r\nThis is a test.)",
			wantErr:      false,
			description:  "Should handle literal with exact size match",
			wantTokens:   2,
			checkContent: true,
		},
		{
			name:        "multiple tokens with literal",
			input:       "(UID 7 BODY {5}\r\nHello FLAGS (\\Seen))",
			wantErr:     false,
			description: "Should handle complex input with literal in middle",
			wantTokens:  6, // UID, 7, BODY, "Hello", FLAGS, container
		},
		{
			name:        "literal with exact boundary",
			input:       "(BODY {3}\r\nabc)",
			wantErr:     false,
			description: "Should handle literal ending exactly at declared size",
			wantTokens:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, err := parseFetchTokens(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("parseFetchTokens() error = nil, wantErr %v", tt.wantErr)
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("parseFetchTokens() error = %v, want error containing %v", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("parseFetchTokens() unexpected error = %v for case: %s", err, tt.description)
					return
				}

				if tt.wantTokens > 0 && len(tokens) != tt.wantTokens {
					t.Errorf("parseFetchTokens() got %d tokens, want %d for case: %s", len(tokens), tt.wantTokens, tt.description)
				}

				if tt.checkContent && len(tokens) >= 2 {
					if tokens[0].Type != TLiteral || tokens[0].Str != "BODY" {
						t.Errorf("parseFetchTokens() first token = %+v, want BODY literal", tokens[0])
					}
					if tt.name == "empty literal {0}" && tokens[1].Type != TAtom {
						t.Errorf("parseFetchTokens() second token type = %v, want TAtom for empty literal", tokens[1].Type)
					}
					if tt.name == "literal with exact size" && (tokens[1].Type != TAtom || tokens[1].Str != "Hello") {
						t.Errorf("parseFetchTokens() second token = %+v, want Hello atom", tokens[1])
					}
				}
			}
		})
	}
}

func TestCalculateTokenEnd(t *testing.T) {
	tests := []struct {
		name        string
		tokenStart  int
		sizeVal     int
		bufferLen   int
		wantEnd     int
		wantErr     bool
		description string
	}{
		{
			name:        "empty literal",
			tokenStart:  10,
			sizeVal:     0,
			bufferLen:   20,
			wantEnd:     9,
			wantErr:     false,
			description: "Empty literal should return tokenStart-1",
		},
		{
			name:        "normal case within bounds",
			tokenStart:  10,
			sizeVal:     5,
			bufferLen:   20,
			wantEnd:     14,
			wantErr:     false,
			description: "Normal case should return tokenStart+sizeVal-1",
		},
		{
			name:        "exact buffer boundary",
			tokenStart:  10,
			sizeVal:     10,
			bufferLen:   20,
			wantEnd:     19,
			wantErr:     false,
			description: "Should handle exact buffer boundary",
		},
		{
			name:        "size exceeds buffer",
			tokenStart:  10,
			sizeVal:     15,
			bufferLen:   20,
			wantEnd:     19,
			wantErr:     false,
			description: "Should truncate to buffer length when size exceeds",
		},
		{
			name:        "tokenStart at buffer end",
			tokenStart:  20,
			sizeVal:     0,
			bufferLen:   20,
			wantEnd:     19,
			wantErr:     false,
			description: "Should handle tokenStart at buffer end with empty literal",
		},
		{
			name:        "tokenStart past buffer end with size",
			tokenStart:  20,
			sizeVal:     5,
			bufferLen:   20,
			wantErr:     true,
			description: "Should error when tokenStart >= bufferLen with non-zero size",
		},
		{
			name:        "tokenStart past buffer end",
			tokenStart:  25,
			sizeVal:     0,
			bufferLen:   20,
			wantEnd:     24,
			wantErr:     false,
			description: "Should handle tokenStart past buffer with empty literal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEnd, err := calculateTokenEnd(tt.tokenStart, tt.sizeVal, tt.bufferLen)

			if tt.wantErr {
				if err == nil {
					t.Errorf("calculateTokenEnd() error = nil, wantErr %v", tt.wantErr)
					return
				}
			} else {
				if err != nil {
					t.Errorf("calculateTokenEnd() unexpected error = %v", err)
					return
				}
				if gotEnd != tt.wantEnd {
					t.Errorf("calculateTokenEnd() = %v, want %v; %s", gotEnd, tt.wantEnd, tt.description)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func TestParseUIDSearchResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    []int
		wantErr bool
	}{
		{
			name:  "basic search response",
			input: "* SEARCH 123 456\r\nA1 OK SEARCH completed\r\n",
			want:  []int{123, 456},
		},
		{
			name: "literal preamble is ignored",
			input: strings.Join([]string{
				"+ Ready for additional command text",
				"* SEARCH 15461 15469 15470 15485 15491 15497",
				"A144 OK UID SEARCH completed",
			}, "\r\n"),
			want: []int{15461, 15469, 15470, 15485, 15491, 15497},
		},
		{
			name:    "no search line",
			input:   "* OK Nothing to see here\r\n",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseUIDSearchResponse(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil result %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseUIDSearchResponse error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestParseFetchResponse(t *testing.T) {
	d := &Dialer{}
	resp := "* 1 FETCH (UID 7 FLAGS (\\Seen))\r\n"
	recs, err := d.ParseFetchResponse(resp)
	if err != nil {
		t.Fatalf("ParseFetchResponse error: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 record got %d", len(recs))
	}
	r := recs[0]
	if len(r) != 4 {
		t.Fatalf("expected 4 tokens got %d", len(r))
	}
	if r[0].Type != TLiteral || r[0].Str != "UID" {
		t.Errorf("unexpected token %#v", r[0])
	}
	if r[1].Type != TNumber || r[1].Num != 7 {
		t.Errorf("unexpected token %#v", r[1])
	}
	if r[2].Type != TLiteral || r[2].Str != "FLAGS" {
		t.Errorf("unexpected token %#v", r[2])
	}
	if r[3].Type != TContainer || len(r[3].Tokens) != 1 || r[3].Tokens[0].Str != "\\Seen" {
		t.Errorf("unexpected token %#v", r[3])
	}
}
