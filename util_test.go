package imap

import (
	"testing"
)

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
