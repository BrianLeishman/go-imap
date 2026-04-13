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

// makeFetchToken creates a Token from the given type and raw string boundaries.
func makeFetchToken(tokenType TType, r string, tokenStart, tokenEnd int) *Token {
	switch tokenType {
	case TQuoted:
		return &Token{Type: tokenType, Str: RemoveSlashes.Replace(string(r[tokenStart : tokenEnd+1]))}
	case TLiteral:
		s := string(r[tokenStart : tokenEnd+1])
		num, err := strconv.Atoi(s)
		if err == nil {
			return &Token{Type: TNumber, Num: num}
		}
		if s == "NIL" {
			return &Token{Type: TNil}
		}
		return &Token{Type: TLiteral, Str: s}
	case TAtom:
		return &Token{Type: tokenType, Str: string(r[tokenStart : tokenEnd+1])}
	case TContainer:
		return &Token{Type: tokenType, Tokens: make([]*Token, 0, 1)}
	default:
		return nil
	}
}

// parseAtomLiteral parses the size and literal data when a '{' has been encountered.
// tokenStart points to the first digit of the size. i is the current position in r.
// Returns the new tokenStart (for literal data), tokenEnd, new i position, and any error.
func parseAtomLiteral(r string, i, tokenStart int) (newTokenStart, tokenEnd, newI int, err error) {
	b := r[i]
	tokenEndOfSize := i // Current 'i' is at '}' or '+'
	if b == '+' {
		i++ // skip '+', now should be '}'
		if i >= len(r) || r[i] != '}' {
			return 0, 0, 0, fmt.Errorf("expected '}' after '+' in literal at char %d in %s", i, r)
		}
	} else if b != '}' {
		// Any non-digit byte after '{<digits>' must be '}' (or '+}').
		// Accepting anything else silently misparses malformed headers
		// like "{5Xabcde" as a 5-byte literal starting after 'X'.
		return 0, 0, 0, fmt.Errorf("expected '}' or '+}' after literal size at char %d, got %q in %s", i, b, r)
	}
	// tokenStart for size was set when '{' was seen. r[tokenStart:tokenEndOfSize] is the size string.
	sizeVal, err := strconv.Atoi(string(r[tokenStart:tokenEndOfSize]))
	if err != nil {
		return 0, 0, 0, fmt.Errorf("TAtom size Atoi failed for '%s': %w", string(r[tokenStart:tokenEndOfSize]), err)
	}

	i++ // Advance 'i' past '}' to the start of actual literal data

	if i < len(r) && r[i] == '\r' {
		i++
	}
	if i < len(r) && r[i] == '\n' {
		i++
	}

	newTokenStart = i // tokenStart is now for the literal data itself

	// Calculate token end position with boundary checks
	tokenEnd, err = calculateTokenEnd(newTokenStart, sizeVal, len(r))
	if err != nil {
		return 0, 0, 0, err
	}

	return newTokenStart, tokenEnd, tokenEnd, nil
}

// fetchParserState holds the mutable state for the token parser.
type fetchParserState struct {
	currentToken TType
	tokenStart   int
	tokenEnd     int
	depth        int
	container    []tokenContainer
}

// handleUnsetByte processes a byte when no token is currently being parsed.
// Returns an error for unmatched ')'.
func (s *fetchParserState) handleUnsetByte(b byte, i int, r string, pushToken func() *Token) error {
	switch {
	case b == '"':
		s.currentToken = TQuoted
		s.tokenStart = i + 1
	case IsLiteral(rune(b)):
		s.currentToken = TLiteral
		s.tokenStart = i
	case b == '{': // Start of a new literal
		s.currentToken = TAtom
		s.tokenStart = i + 1 // tokenStart for the size digits
	case b == '(':
		s.currentToken = TContainer
		t := pushToken() // push container token
		s.depth++
		// Grow container stack if needed
		if s.depth >= len(s.container) {
			newContainer := make([]tokenContainer, s.depth*2)
			copy(newContainer, s.container)
			s.container = newContainer
		}
		s.container[s.depth] = &t.Tokens
	case b == ')':
		if s.depth == 0 { // Unmatched ')'
			return fmt.Errorf("unmatched ')' at char %d in %s", i, r)
		}
		pushToken() // push any pending token before closing container
		s.depth--
	}
	return nil
}

// handleActiveToken processes a byte when a token is actively being parsed.
// Returns (newI, skip, err) where skip=true means the caller should skip the unset check.
func (s *fetchParserState) handleActiveToken(r string, b byte, i int, pushToken func() *Token) (int, bool, error) {
	switch s.currentToken {
	case TQuoted:
		switch b {
		case '"':
			s.tokenEnd = i - 1
			pushToken()
			return i, true, nil
		case '\\':
			return i + 1, true, nil
		}
	case TLiteral:
		if !IsLiteral(rune(b)) {
			s.tokenEnd = i - 1
			pushToken()
		}
	case TAtom:
		if !unicode.IsDigit(rune(b)) {
			newTokenStart, tokenEnd, newI, err := parseAtomLiteral(r, i, s.tokenStart)
			if err != nil {
				return 0, false, err
			}
			s.tokenStart = newTokenStart
			s.tokenEnd = tokenEnd
			i = newI
			pushToken()
			// parseAtomLiteral consumed the terminating '}' (or '+}')
			// plus the literal body, so `b` no longer corresponds to
			// the current position — skip the unset re-dispatch.
			return i, true, nil
		}
	}
	return i, false, nil
}

// parseFetchTokens parses IMAP FETCH response tokens
func parseFetchTokens(r string) ([]*Token, error) {
	tokens := make([]*Token, 0)

	st := fetchParserState{
		currentToken: TUnset,
		container:    make([]tokenContainer, 4),
	}
	st.container[0] = &tokens

	pushToken := func() *Token {
		t := makeFetchToken(st.currentToken, r, st.tokenStart, st.tokenEnd)
		if t != nil {
			*st.container[st.depth] = append(*st.container[st.depth], t)
		}
		st.currentToken = TUnset
		return t
	}

	l := len(r)
	i := 0
	for i < l {
		b := r[i]

		newI, skip, err := st.handleActiveToken(r, b, i, pushToken)
		if err != nil {
			return nil, err
		}
		i = newI

		if !skip && st.currentToken == TUnset {
			if err := st.handleUnsetByte(b, i, r, pushToken); err != nil {
				return nil, err
			}
		}

		if st.depth < 0 {
			break
		}
		i++
		if i >= l && st.currentToken != TUnset {
			st.tokenEnd = l - 1
			pushToken()
		}
	}

	if st.depth != 0 {
		return nil, fmt.Errorf("mismatched parentheses, depth %d at end of parsing %s", st.depth, r)
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

// skipIMAPLiteralInContent attempts to skip an IMAP literal ({NNN} or {NNN+}) at position i in s.
// If a valid literal is found, returns the new position after the literal data, skipped=true, and any error.
// If the '{' is not a valid literal start, returns the original i, skipped=false, nil.
func skipIMAPLiteralInContent(s string, i int) (newI int, skipped bool, err error) {
	j := i + 1
	for j < len(s) && unicode.IsDigit(rune(s[j])) {
		j++
	}
	// Support LITERAL+ (RFC 7888): {NNN+} syntax
	if j < len(s) && s[j] == '+' {
		j++
	}
	if j <= i+1 || j >= len(s) || s[j] != '}' {
		return i, false, nil
	}
	sizeEnd := j
	if s[sizeEnd-1] == '+' {
		sizeEnd--
	}
	size, err := strconv.Atoi(s[i+1 : sizeEnd])
	if err != nil {
		return 0, false, fmt.Errorf("parse literal size %q: %w", s[i+1:sizeEnd], err)
	}

	i = j + 1
	if i < len(s) && s[i] == '\r' {
		i++
	}
	if i < len(s) && s[i] == '\n' {
		i++
	}
	if i+size > len(s) {
		return 0, false, fmt.Errorf("literal size %d exceeds remaining buffer (%d)", size, len(s)-i)
	}
	return i + size, true, nil
}

// skipQuotedRun advances past all bytes inside a quoted string starting at position i.
// The position i should be the byte after the opening '"'.
// Returns the position after the closing '"'.
func skipQuotedRun(s string, i int) int {
	for i < len(s) {
		switch s[i] {
		case '\\':
			if i+1 < len(s) {
				i += 2
				continue
			}
		case '"':
			return i + 1
		}
		i++
	}
	return i
}

// contentEndStep processes one byte in the parenthesis-balanced scan.
// Returns (newI, newDepth, done, endPos, err). When done=true, endPos is the result.
func contentEndStep(s string, i, depth int) (newI, newDepth int, done bool, endPos int, err error) {
	switch s[i] {
	case '"':
		return skipQuotedRun(s, i+1), depth, false, 0, nil
	case '{':
		ni, skipped, litErr := skipIMAPLiteralInContent(s, i)
		if litErr != nil {
			return 0, depth, true, 0, litErr
		}
		if skipped {
			return ni, depth, false, 0, nil
		}
	case '(':
		depth++
	case ')':
		if depth == 0 {
			return 0, depth, true, 0, fmt.Errorf("unmatched ')' at char %d", i)
		}
		depth--
		if depth == 0 {
			return i + 1, depth, true, i + 1, nil
		}
	}
	return i + 1, depth, false, 0, nil
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
	for i < len(s) {
		var done bool
		var endPos int
		var err error
		i, depth, done, endPos, err = contentEndStep(s, i, depth)
		if err != nil {
			return 0, err
		}
		if done {
			return endPos, nil
		}
	}

	return 0, fmt.Errorf("unterminated FETCH response (unbalanced parentheses)")
}

// parseSingleFetchLine parses a single "* N FETCH ..." line and returns the tokens.
func parseSingleFetchLine(line string) ([]*Token, error) {
	fetchContentStart, fetchContentEnd, err := extractFetchContent(line, 0)
	if err != nil {
		return nil, err
	}
	fetchContent := line[fetchContentStart:fetchContentEnd]
	tokens, parseErr := parseFetchTokens(fetchContent)
	if parseErr != nil {
		return nil, fmt.Errorf("token parsing failed for line part [%s] from original line [%s]: %w", fetchContent, line, parseErr)
	}
	return tokens, nil
}

// extractFetchContent locates the FETCH content boundaries within body starting at start.
// Returns fetchContentStart, fetchContentEnd, and any error.
func extractFetchContent(body string, start int) (fetchContentStart, fetchContentEnd int, err error) {
	lineSnippet := func() string {
		return strings.TrimSpace(body[start:findLineEnd(body, start)])
	}
	if !strings.HasPrefix(body[start:], "* ") {
		return 0, 0, fmt.Errorf("unable to parse Fetch line (expected '* ' prefix, regex mismatch?): %#v", lineSnippet())
	}

	restStart := start + 2
	rest := body[restStart:]
	idx := strings.IndexByte(rest, ' ')
	if idx == -1 {
		return 0, 0, fmt.Errorf("unable to parse Fetch line (no space after seq number, regex mismatch?): %#v", lineSnippet())
	}

	seqNumStr := rest[:idx]
	if _, convErr := strconv.Atoi(seqNumStr); convErr != nil {
		return 0, 0, fmt.Errorf("unable to parse Fetch line (invalid seq num %s): %#v: %w", seqNumStr, lineSnippet(), convErr)
	}

	restAfterSeqStart := restStart + idx + 1
	for restAfterSeqStart < len(body) && body[restAfterSeqStart] == ' ' {
		restAfterSeqStart++
	}
	if !strings.HasPrefix(body[restAfterSeqStart:], "FETCH ") {
		return 0, 0, fmt.Errorf("unable to parse Fetch line (expected 'FETCH ' prefix after seq num, regex mismatch?): %#v", lineSnippet())
	}
	fetchContentStart = restAfterSeqStart + len("FETCH ")
	fetchContentEnd, endErr := findFetchContentEnd(body, fetchContentStart)
	if endErr != nil {
		return 0, 0, fmt.Errorf("failed to locate end of FETCH response from line [%s]: %w", lineSnippet(), endErr)
	}
	return fetchContentStart, fetchContentEnd, nil
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
		if !strings.HasPrefix(trimmedResponseBody, "* ") {
			return records, nil
		}
		tokens, parseErr := parseSingleFetchLine(trimmedResponseBody)
		if parseErr != nil {
			return nil, parseErr
		}
		records = append(records, tokens)
		return records, nil
	}

	parsedUntil := 0
	for _, loc := range locs {
		start := loc[0]
		if start < parsedUntil {
			continue
		}

		fetchContentStart, fetchContentEnd, extractErr := extractFetchContent(trimmedResponseBody, start)
		if extractErr != nil {
			return nil, extractErr
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
		if len(line) > 2 && line[:2] == "* " {
			upper := strings.ToUpper(line)
			if strings.Contains(upper, "ESEARCH") {
				// If MAX keyword is present but didn't match \d+, that's malformed
				if strings.Contains(upper, " MAX ") {
					return 0, fmt.Errorf("malformed ESEARCH MAX value in: %q", line)
				}
				// ESEARCH present without MAX means empty result set (RFC 4731)
				return 0, nil
			}
		}
	}

	return 0, fmt.Errorf("no ESEARCH line. rfc4731 not supported?")
}

// IsLiteral checks if a rune is valid for a literal token.
//
// This matches RFC 3501 ATOM-CHAR (plus '\' and ']' for flag and
// BODY[...] syntax): any character except atom-specials
// ("(", ")", "{", SP, CTL, list-wildcards, quoted-specials). Custom
// IMAP keyword flags commonly use '$', '-', '_', '+', etc. — rejecting
// those chars caused flags like "$X-ME-Annot-2" (Fastmail) to be split
// across multiple tokens. See issue #90.
func IsLiteral(b rune) bool {
	switch b {
	case '(', ')', '{', ' ', '%', '*', '"':
		return false
	}
	if b < 0x20 || b == 0x7f {
		return false
	}
	return true
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
