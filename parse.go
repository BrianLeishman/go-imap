package imap

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

const (
	nl         = "\r\n"
	TimeFormat = "_2-Jan-2006 15:04:05 -0700"
)

var (
	atom             = regexp.MustCompile(`{\d+\+?}$`)
	fetchLineStartRE = regexp.MustCompile(`(?m)^\* \d+ FETCH`)
	searchMaxUIDRE   = regexp.MustCompile(`(?i)\* ESEARCH .* MAX (\d+)`)
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
			default: // Should be '}' or '+}' (LITERAL+, RFC 7888)
				tokenEndOfSize := i // Current 'i' is at '}' or '+'
				if b == '+' {
					i++ // skip '+', now should be '}'
					if i >= len(r) || r[i] != '}' {
						return nil, fmt.Errorf("expected '}' after '+' in literal at char %d in %s", i, r)
					}
				}
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

func findLineEnd(s string, start int) int {
	lineEndRel := strings.Index(s[start:], nl)
	if lineEndRel == -1 {
		return len(s)
	}
	return start + lineEndRel
}

func findFetchContentEnd(s string, fetchContentStart int) (int, error) {
	if fetchContentStart >= len(s) {
		return len(s), nil
	}

	i := fetchContentStart
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	if i >= len(s) {
		return len(s), nil
	}
	if s[i] != '(' {
		return findLineEnd(s, fetchContentStart), nil
	}

	depth := 0
	inQuoted := false
	for i < len(s) {
		b := s[i]

		if inQuoted {
			switch b {
			case '\\':
				if i+1 < len(s) {
					i += 2
					continue
				}
			case '"':
				inQuoted = false
			}
			i++
			continue
		}

		switch b {
		case '"':
			inQuoted = true
			i++
			continue
		case '{':
			j := i + 1
			for j < len(s) && unicode.IsDigit(rune(s[j])) {
				j++
			}
			// Support LITERAL+ (RFC 7888): {NNN+} syntax
			if j < len(s) && s[j] == '+' {
				j++
			}
			if j > i+1 && j < len(s) && s[j] == '}' {
				sizeEnd := j
				if s[sizeEnd-1] == '+' {
					sizeEnd--
				}
				size, err := strconv.Atoi(s[i+1 : sizeEnd])
				if err != nil {
					return 0, fmt.Errorf("parse literal size %q: %w", s[i+1:sizeEnd], err)
				}

				i = j + 1
				if i < len(s) && s[i] == '\r' {
					i++
				}
				if i < len(s) && s[i] == '\n' {
					i++
				}
				if i+size > len(s) {
					return 0, fmt.Errorf("literal size %d exceeds remaining buffer (%d)", size, len(s)-i)
				}
				i += size
				continue
			}
		case '(':
			depth++
			i++
			continue
		case ')':
			if depth == 0 {
				return 0, fmt.Errorf("unmatched ')' at char %d", i)
			}
			depth--
			i++
			if depth == 0 {
				return i, nil
			}
			continue
		}

		i++
	}

	return 0, fmt.Errorf("unterminated FETCH response (unbalanced parentheses)")
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

	parsedUntil := 0
	for _, loc := range locs {
		start := loc[0]
		if start < parsedUntil {
			continue
		}

		if !strings.HasPrefix(trimmedResponseBody[start:], "* ") {
			return nil, fmt.Errorf("unable to parse Fetch line (expected '* ' prefix, regex mismatch?): %#v", strings.TrimSpace(trimmedResponseBody[start:findLineEnd(trimmedResponseBody, start)]))
		}

		restStart := start + 2
		rest := trimmedResponseBody[restStart:]
		idx := strings.IndexByte(rest, ' ')
		if idx == -1 {
			return nil, fmt.Errorf("unable to parse Fetch line (no space after seq number, regex mismatch?): %#v", strings.TrimSpace(trimmedResponseBody[start:findLineEnd(trimmedResponseBody, start)]))
		}

		seqNumStr := rest[:idx]
		if _, convErr := strconv.Atoi(seqNumStr); convErr != nil {
			return nil, fmt.Errorf("unable to parse Fetch line (invalid seq num %s): %#v: %w", seqNumStr, strings.TrimSpace(trimmedResponseBody[start:findLineEnd(trimmedResponseBody, start)]), convErr)
		}

		restAfterSeqStart := restStart + idx + 1
		for restAfterSeqStart < len(trimmedResponseBody) && trimmedResponseBody[restAfterSeqStart] == ' ' {
			restAfterSeqStart++
		}
		if !strings.HasPrefix(trimmedResponseBody[restAfterSeqStart:], "FETCH ") {
			return nil, fmt.Errorf("unable to parse Fetch line (expected 'FETCH ' prefix after seq num, regex mismatch?): %#v", strings.TrimSpace(trimmedResponseBody[start:findLineEnd(trimmedResponseBody, start)]))
		}
		fetchContentStart := restAfterSeqStart + len("FETCH ")
		fetchContentEnd, endErr := findFetchContentEnd(trimmedResponseBody, fetchContentStart)
		if endErr != nil {
			return nil, fmt.Errorf("failed to locate end of FETCH response from line [%s]: %w", strings.TrimSpace(trimmedResponseBody[start:findLineEnd(trimmedResponseBody, start)]), endErr)
		}

		currentLineToProcess := strings.TrimSpace(trimmedResponseBody[start:fetchContentEnd])
		fetchContent := trimmedResponseBody[fetchContentStart:fetchContentEnd]
		tokens, err := parseFetchTokens(fetchContent)
		if err != nil {
			return nil, fmt.Errorf("token parsing failed for line part [%s] from original line [%s]: %w", fetchContent, currentLineToProcess, err)
		}
		records = append(records, tokens)
		parsedUntil = fetchContentEnd
	}
	return records, nil
}

// parseUIDSearchResponse parses UID SEARCH command responses
func parseUIDSearchResponse(r string) ([]int, error) {
	normalized := strings.ReplaceAll(r, nl, "\n")
	for rawLine := range strings.SplitSeq(normalized, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "*" || !strings.EqualFold(fields[1], "SEARCH") {
			continue
		}

		uids := make([]int, 0, len(fields)-2)
		for _, f := range fields[2:] {
			u, err := strconv.Atoi(f)
			if err != nil {
				return nil, fmt.Errorf("parse uid %q: %w", f, err)
			}
			uids = append(uids, u)
		}
		return uids, nil
	}

	return nil, fmt.Errorf("invalid response: %q", strings.TrimSpace(r))
}

// Parse SEARCH RETURN(MAX) command response
//
// Expected response format (RFC 4731)
//
//	C: A285 UID SEARCH RETURN (MAX) 1:5000
//	S: * ESEARCH (TAG "A285") UID MAX 3800
//	S: A285 OK SEARCH completed
//
// When the mailbox is empty, RFC 4731 omits MAX from the ESEARCH line:
//
//	S: * ESEARCH (TAG "A285") UID
//
// In that case this function returns 0, nil.
// ref https://www.rfc-editor.org/rfc/rfc4731.html#page-2
func parseMaxUIDSearchResponse(r string) (int, error) {
	normalized := strings.ReplaceAll(r, nl, "\n")
	for rawLine := range strings.SplitSeq(normalized, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		if matches := searchMaxUIDRE.FindStringSubmatch(line); len(matches) > 1 {
			maxUID, err := strconv.Atoi(matches[1])
			if err != nil {
				return 0, fmt.Errorf("parse max uid %q: %w", matches[1], err)
			}
			return maxUID, nil
		}

		// Check for ESEARCH line without a valid MAX capture
		upper := strings.ToUpper(line)
		if len(line) > 2 && strings.EqualFold(line[:2], "* ") && strings.Contains(upper, "ESEARCH") {
			// If MAX keyword is present but didn't match \d+, that's malformed
			if strings.Contains(upper, " MAX ") {
				return 0, fmt.Errorf("malformed ESEARCH MAX value in: %q", line)
			}
			// ESEARCH present without MAX means empty result set (RFC 4731)
			return 0, nil
		}
	}

	return 0, fmt.Errorf("no ESEARCH line. rfc4731 not supported?")
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
func (d *Dialer) CheckType(token *Token, acceptableTypes []TType, tks []*Token, loc string, v ...any) (err error) {
	if slices.Contains(acceptableTypes, token.Type) {
		return nil
	}

	var b strings.Builder
	for i, a := range acceptableTypes {
		if i != 0 {
			b.WriteByte('|')
		}
		b.WriteString(GetTokenName(a))
	}
	return fmt.Errorf("IMAP%d:%s: expected %s token %s, got %+v in %v", d.ConnNum, d.Folder, b.String(), fmt.Sprintf(loc, v...), token, tks)
}
