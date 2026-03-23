package imap

import (
	"testing"
)

func TestDropNl(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{"crlf", []byte("hello\r\n"), "hello"},
		{"lf only", []byte("hello\n"), "hello"},
		{"no newline", []byte("hello"), "hello"},
		{"empty", []byte{}, ""},
		{"just lf", []byte("\n"), ""},
		{"just crlf", []byte("\r\n"), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := string(dropNl(tt.input))
			if got != tt.expected {
				t.Errorf("dropNl(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestMakeIMAPLiteral(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"test", "{4}\r\ntest"},
		{"тест", "{8}\r\nтест"},
		{"测试", "{6}\r\n测试"},
		{"😀👍", "{8}\r\n😀👍"},
		{"Prüfung", "{8}\r\nPrüfung"},
		{"", "{0}\r\n"},
	}

	for _, test := range tests {
		got := MakeIMAPLiteral(test.input)
		if got != test.expected {
			t.Errorf("MakeIMAPLiteral(%q) = %q, want %q", test.input, got, test.expected)
		}
	}
}
