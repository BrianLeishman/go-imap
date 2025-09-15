package imap

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

const (
	nl         = "\r\n"
	TimeFormat = "_2-Jan-2006 15:04:05 -0700"
)

var (
	atom             = regexp.MustCompile(`{\d+}$`)
	fetchLineStartRE = regexp.MustCompile(`(?m)^\* \d+ FETCH`)
)

// Token represents a parsed IMAP token
type Token struct {
	Type   TType
	Str    string
	Num    int
	Tokens []*Token
}

// TType represents the type of an IMAP token
type TType uint8

const (
	TUnset TType = iota
	TAtom
	TNumber
	TLiteral
	TQuoted
	TNil
	TContainer
)

type tokenContainer *[]*Token

// calculateTokenEnd calculates the end position of a literal token based on size and buffer constraints
func calculateTokenEnd(tokenStart, sizeVal, bufferLen int) (int, error) {
	switch {
	case tokenStart >= bufferLen:
		if sizeVal == 0 {
			return tokenStart - 1, nil // Results in empty string for r[tokenStart:tokenEnd+1]
		}
		return 0, fmt.Errorf("TAtom: literal size %d but tokenStart %d is at/past end of buffer %d", sizeVal, tokenStart, bufferLen)
	case tokenStart+sizeVal > bufferLen:
		return bufferLen - 1, nil // Taking available data
	default:
		return tokenStart + sizeVal - 1, nil // Normal case: sizeVal fits
	}
}

// parseFetchTokens parses IMAP FETCH response tokens
func parseFetchTokens(r string) ([]*Token, error) {
	tokens := make([]*Token, 0)

	currentToken := TUnset
	tokenStart := 0
	tokenEnd := 0
	depth := 0
	container := make([]tokenContainer, 4)
	container[0] = &tokens

	pushToken := func() *Token {
		var t *Token
		switch currentToken {
		case TQuoted:
			t = &Token{
				Type: currentToken,
				Str:  RemoveSlashes.Replace(string(r[tokenStart : tokenEnd+1])),
			}
		case TLiteral:
			s := string(r[tokenStart : tokenEnd+1])
			num, err := strconv.Atoi(s)
			if err == nil {
				t = &Token{
					Type: TNumber,
					Num:  num,
				}
			} else {
				if s == "NIL" {
					t = &Token{
						Type: TNil,
					}
				} else {
					t = &Token{
						Type: TLiteral,
						Str:  s,
					}
				}
			}
		case TAtom:
			t = &Token{
				Type: currentToken,
				Str:  string(r[tokenStart : tokenEnd+1]),
			}
		case TContainer:
			t = &Token{
				Type:   currentToken,
				Tokens: make([]*Token, 0, 1),
			}
		}

		if t != nil {
			*container[depth] = append(*container[depth], t)
		}
		currentToken = TUnset

		return t
	}

	l := len(r)
	i := 0
	for i < l {
		b := r[i]

		switch currentToken {
		case TQuoted:
			switch b {
			case '"':
				tokenEnd = i - 1
				pushToken()
				goto Cont
			case '\\':
				i++
				goto Cont
			}
		case TLiteral:
			switch {
			case IsLiteral(rune(b)):
			default:
				tokenEnd = i - 1
				pushToken()
			}
		case TAtom:
			switch {
			case unicode.IsDigit(rune(b)):
				// Still accumulating digits for size, main loop's i++ will advance
			default: // Should be '}'
				tokenEndOfSize := i // Current 'i' is at '}'
				// tokenStart for size was set when '{' was seen. r[tokenStart:tokenEndOfSize] is the size string.
				sizeVal, err := strconv.Atoi(string(r[tokenStart:tokenEndOfSize]))
				if err != nil {
					return nil, fmt.Errorf("TAtom size Atoi failed for '%s': %w", string(r[tokenStart:tokenEndOfSize]), err)
				}

				i++ // Advance 'i' past '}' to the start of actual literal data

				if i < len(r) && r[i] == '\r' {
					i++
				}
				if i < len(r) && r[i] == '\n' {
					i++
				}

				tokenStart = i // tokenStart is now for the literal data itself

				// Calculate token end position with boundary checks
				tokenEnd, err = calculateTokenEnd(tokenStart, sizeVal, len(r))
				if err != nil {
					return nil, err
				}

				i = tokenEnd // Move main loop cursor to the end of the literal data
				pushToken()  // Push the TAtom token
			}
		}

		if currentToken == TUnset { // If no token is being actively parsed
			switch {
			case b == '"':
				currentToken = TQuoted
				tokenStart = i + 1
			case IsLiteral(rune(b)):
				currentToken = TLiteral
				tokenStart = i
			case b == '{': // Start of a new literal
				currentToken = TAtom
				tokenStart = i + 1 // tokenStart for the size digits
			case b == '(':
				currentToken = TContainer
				t := pushToken() // push any pending token before starting container
				depth++
				// Grow container stack if needed
				if depth >= len(container) {
					newContainer := make([]tokenContainer, depth*2)
					copy(newContainer, container)
					container = newContainer
				}
				container[depth] = &t.Tokens
			case b == ')':
				if depth == 0 { // Unmatched ')'
					return nil, fmt.Errorf("unmatched ')' at char %d in %s", i, r)
				}
				pushToken() // push any pending token before closing container
				depth--
			}
		}

	Cont:
		if depth < 0 {
			break
		}
		i++
		if i >= l { // If we've processed all characters or gone past
			if currentToken != TUnset { // Only push if there's a pending token
				tokenEnd = l - 1 // The last character is at index l-1
				pushToken()
			}
		}
	}

	if depth != 0 {
		return nil, fmt.Errorf("mismatched parentheses, depth %d at end of parsing %s", depth, r)
	}

	if len(tokens) == 1 && tokens[0].Type == TContainer {
		tokens = tokens[0].Tokens
	}

	return tokens, nil
}

// ParseFetchResponse parses a multi-line FETCH response
func (d *Dialer) ParseFetchResponse(responseBody string) (records [][]*Token, err error) {
	records = make([][]*Token, 0)
	trimmedResponseBody := strings.TrimSpace(responseBody)
	if trimmedResponseBody == "" {
		return records, nil
	}

	locs := fetchLineStartRE.FindAllStringIndex(trimmedResponseBody, -1)

	if locs == nil {
		// No FETCH lines found by regex.
		// Try to parse as a single line if it starts with "* ".
		if strings.HasPrefix(trimmedResponseBody, "* ") {
			currentLineToProcess := trimmedResponseBody
			// Standard parsing logic for a single line
			if !strings.HasPrefix(currentLineToProcess, "* ") {
				return nil, fmt.Errorf("unable to parse Fetch line (expected '* ' prefix): %#v", currentLineToProcess)
			}
			rest := currentLineToProcess[2:]
			idx := strings.IndexByte(rest, ' ')
			if idx == -1 {
				return nil, fmt.Errorf("unable to parse Fetch line (no space after seq number): %#v", currentLineToProcess)
			}
			seqNumStr := rest[:idx]
			if _, convErr := strconv.Atoi(seqNumStr); convErr != nil {
				return nil, fmt.Errorf("unable to parse Fetch line (invalid seq num %s): %#v: %w", seqNumStr, currentLineToProcess, convErr)
			}
			rest = strings.TrimSpace(rest[idx+1:])
			if !strings.HasPrefix(rest, "FETCH ") {
				return nil, fmt.Errorf("unable to parse Fetch line (expected 'FETCH ' prefix after seq num): %#v", currentLineToProcess)
			}
			fetchContent := rest[len("FETCH "):]
			tokens, parseErr := parseFetchTokens(fetchContent)
			if parseErr != nil {
				return nil, fmt.Errorf("token parsing failed for line part [%s] from original line [%s]: %w", fetchContent, currentLineToProcess, parseErr)
			}
			records = append(records, tokens)
			return records, nil
		}
		// If not starting with "* " and no FETCH lines found by regex, return empty or error.
		return records, nil
	}

	for i, loc := range locs {
		start := loc[0]
		end := len(trimmedResponseBody)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		line := trimmedResponseBody[start:end]
		currentLineToProcess := strings.TrimSpace(line)

		if len(currentLineToProcess) == 0 {
			continue
		}

		if !strings.HasPrefix(currentLineToProcess, "* ") {
			return nil, fmt.Errorf("unable to parse Fetch line (expected '* ' prefix, regex mismatch?): %#v", currentLineToProcess)
		}
		rest := currentLineToProcess[2:]
		idx := strings.IndexByte(rest, ' ')
		if idx == -1 {
			return nil, fmt.Errorf("unable to parse Fetch line (no space after seq number, regex mismatch?): %#v", currentLineToProcess)
		}

		seqNumStr := rest[:idx]
		if _, convErr := strconv.Atoi(seqNumStr); convErr != nil {
			return nil, fmt.Errorf("unable to parse Fetch line (invalid seq num %s): %#v: %w", seqNumStr, currentLineToProcess, convErr)
		}

		rest = strings.TrimSpace(rest[idx+1:])
		if !strings.HasPrefix(rest, "FETCH ") {
			return nil, fmt.Errorf("unable to parse Fetch line (expected 'FETCH ' prefix after seq num, regex mismatch?): %#v", currentLineToProcess)
		}

		fetchContent := rest[len("FETCH "):]
		tokens, err := parseFetchTokens(fetchContent)
		if err != nil {
			return nil, fmt.Errorf("token parsing failed for line part [%s] from original line [%s]: %w", fetchContent, currentLineToProcess, err)
		}
		records = append(records, tokens)
	}
	return records, nil
}

// parseUIDSearchResponse parses UID SEARCH command responses
func parseUIDSearchResponse(r string) ([]int, error) {
	if idx := strings.Index(r, nl); idx != -1 {
		r = r[:idx]
	}
	fields := strings.Fields(r)
	if len(fields) >= 2 && fields[0] == "*" && fields[1] == "SEARCH" {
		uids := make([]int, 0, len(fields)-2)
		for _, f := range fields[2:] {
			u, err := strconv.Atoi(f)
			if err != nil {
				return nil, err
			}
			uids = append(uids, u)
		}
		return uids, nil
	}
	return nil, fmt.Errorf("invalid response: %q", r)
}

// IsLiteral checks if a rune is valid for a literal token
func IsLiteral(b rune) bool {
	switch {
	case unicode.IsDigit(b),
		unicode.IsLetter(b),
		b == '\\',
		b == '.',
		b == '[',
		b == ']':
		return true
	}
	return false
}

// GetTokenName returns the string name of a token type
func GetTokenName(tokenType TType) string {
	switch tokenType {
	case TUnset:
		return "TUnset"
	case TAtom:
		return "TAtom"
	case TNumber:
		return "TNumber"
	case TLiteral:
		return "TLiteral"
	case TQuoted:
		return "TQuoted"
	case TNil:
		return "TNil"
	case TContainer:
		return "TContainer"
	}
	return ""
}

// String returns a string representation of a Token
func (t Token) String() string {
	tokenType := GetTokenName(t.Type)
	switch t.Type {
	case TUnset, TNil:
		return tokenType
	case TAtom, TQuoted:
		return fmt.Sprintf("(%s, len %d, chars %d %#v)", tokenType, len(t.Str), len([]rune(t.Str)), t.Str)
	case TNumber:
		return fmt.Sprintf("(%s %d)", tokenType, t.Num)
	case TLiteral:
		return fmt.Sprintf("(%s %s)", tokenType, t.Str)
	case TContainer:
		return fmt.Sprintf("(%s children: %s)", tokenType, t.Tokens)
	}
	return ""
}

// CheckType validates that a token is one of the acceptable types
func (d *Dialer) CheckType(token *Token, acceptableTypes []TType, tks []*Token, loc string, v ...interface{}) (err error) {
	ok := false
	for _, a := range acceptableTypes {
		if token.Type == a {
			ok = true
			break
		}
	}
	if !ok {
		types := ""
		for i, a := range acceptableTypes {
			if i != 0 {
				types += "|"
			}
			types += GetTokenName(a)
		}
		err = fmt.Errorf("IMAP%d:%s: expected %s token %s, got %+v in %v", d.ConnNum, d.Folder, types, fmt.Sprintf(loc, v...), token, tks)
	}

	return err
}
