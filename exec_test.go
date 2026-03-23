package imap

import (
	"bufio"
	"strings"
	"testing"
)

func TestReadLiterals_NoLiteral(t *testing.T) {
	t.Parallel()
	r := bufio.NewReader(strings.NewReader(""))
	line := []byte("* 1 FETCH (UID 42)\r\n")
	result, err := readLiterals(r, line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != string(line) {
		t.Errorf("expected %q, got %q", string(line), string(result))
	}
}

func TestReadLiterals_WithLiteral(t *testing.T) {
	t.Parallel()
	// Simulate a literal: line ends with {5}\r\n and then 5 bytes follow plus another line
	line := []byte("* 1 FETCH (BODY[] {5}\r\n")
	continuation := "helloworld\r\n"
	r := bufio.NewReader(strings.NewReader(continuation))

	result, err := readLiterals(r, line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should contain original line + 5 literal bytes + remaining line
	expected := string(line) + "hello" + "world\r\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestReadLiterals_LiteralPlus(t *testing.T) {
	t.Parallel()
	// LITERAL+ uses {NNN+} syntax
	line := []byte("* 1 FETCH (BODY[] {3+}\r\n")
	continuation := "abcrest\r\n"
	r := bufio.NewReader(strings.NewReader(continuation))

	result, err := readLiterals(r, line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := string(line) + "abc" + "rest\r\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestReadLiterals_NoMatch(t *testing.T) {
	t.Parallel()
	// {notanumber} doesn't match the atom regex {\d+\+?}$, so line is returned as-is
	line := []byte("* 1 FETCH (BODY[] {notanumber}\r\n")
	r := bufio.NewReader(strings.NewReader(""))

	result, err := readLiterals(r, line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != string(line) {
		t.Errorf("expected line unchanged, got %q", string(result))
	}
}

func TestReadLiterals_ReadError(t *testing.T) {
	t.Parallel()
	// {5} matches but there's not enough data to read
	line := []byte("* 1 FETCH (BODY[] {5}\r\n")
	r := bufio.NewReader(strings.NewReader("ab")) // only 2 bytes, need 5

	_, err := readLiterals(r, line)
	if err == nil {
		t.Fatal("expected error for short read")
	}
}
