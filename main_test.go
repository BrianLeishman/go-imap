package imap

import (
	"reflect"
	"testing"
)

func TestParseUIDSearchResponse(t *testing.T) {
	resp := "* SEARCH 123 456\r\nA1 OK SEARCH completed\r\n"
	got, err := parseUIDSearchResponse(resp)
	if err != nil {
		t.Fatalf("parseUIDSearchResponse error: %v", err)
	}
	want := []int{123, 456}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
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
