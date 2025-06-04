package imap

import (
	"io"
	"mime"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/net/html/charset"
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

func parseRecords(d *Dialer, records [][]*Token) (map[int]*Email, error) {
	emails := make(map[int]*Email, len(records))
	CharsetReader := func(label string, input io.Reader) (io.Reader, error) {
		label = strings.Replace(label, "windows-", "cp", -1)
		encoding, _ := charset.Lookup(label)
		return encoding.NewDecoder().Reader(input), nil
	}
	dec := mime.WordDecoder{CharsetReader: CharsetReader}
	for _, tks := range records {
		e := &Email{}
		skip := 0
		for i, t := range tks {
			if skip > 0 {
				skip--
				continue
			}
			if err := d.CheckType(t, []TType{TLiteral}, tks, "in root"); err != nil {
				return nil, err
			}
			switch t.Str {
			case "ENVELOPE":
				if err := d.CheckType(tks[i+1], []TType{TContainer}, tks, "after ENVELOPE"); err != nil {
					return nil, err
				}
				if err := d.CheckType(tks[i+1].Tokens[EDate], []TType{TQuoted, TNil}, tks, "for ENVELOPE[%d]", EDate); err != nil {
					return nil, err
				}
				if err := d.CheckType(tks[i+1].Tokens[ESubject], []TType{TQuoted, TAtom, TNil}, tks, "for ENVELOPE[%d]", ESubject); err != nil {
					return nil, err
				}
				e.Subject, _ = dec.DecodeHeader(tks[i+1].Tokens[ESubject].Str)
				for _, a := range []struct {
					dest  *EmailAddresses
					pos   uint8
					debug string
				}{
					{&e.To, ETo, "TO"},
				} {
					if tks[i+1].Tokens[a.pos].Type != TNil {
						if err := d.CheckType(tks[i+1].Tokens[a.pos], []TType{TNil, TContainer}, tks, "for ENVELOPE[%d]", a.pos); err != nil {
							return nil, err
						}
						*a.dest = make(map[string]string, len(tks[i+1].Tokens[a.pos].Tokens))
						for j, t := range tks[i+1].Tokens[a.pos].Tokens {
							if err := d.CheckType(t.Tokens[EEName], []TType{TQuoted, TAtom, TNil}, tks, "for %s[%d][%d]", a.debug, j, EEName); err != nil {
								return nil, err
							}
							if err := d.CheckType(t.Tokens[EEMailbox], []TType{TQuoted, TAtom, TNil}, tks, "for %s[%d][%d]", a.debug, j, EEMailbox); err != nil {
								return nil, err
							}
							if err := d.CheckType(t.Tokens[EEHost], []TType{TQuoted, TAtom, TNil}, tks, "for %s[%d][%d]", a.debug, j, EEHost); err != nil {
								return nil, err
							}
							name, err := dec.DecodeHeader(t.Tokens[EEName].Str)
							if err != nil {
								return nil, err
							}
							mailbox, err := dec.DecodeHeader(t.Tokens[EEMailbox].Str)
							if err != nil {
								return nil, err
							}
							host, err := dec.DecodeHeader(t.Tokens[EEHost].Str)
							if err != nil {
								return nil, err
							}
							(*a.dest)[strings.ToLower(mailbox+"@"+host)] = name
						}
					}
				}
				skip++
			case "UID":
				if err := d.CheckType(tks[i+1], []TType{TNumber}, tks, "after UID"); err != nil {
					return nil, err
				}
				e.UID = tks[i+1].Num
				skip++
			}
		}
		emails[e.UID] = e
	}
	return emails, nil
}

func TestEnvelopeAtomAddress(t *testing.T) {
	name := "CBJ SAP SUPPORT INSIGHT"
	env := &Token{Type: TContainer, Tokens: []*Token{
		{Type: TNil}, // date
		{Type: TAtom, Str: "sub"},
		{Type: TNil},
		{Type: TNil},
		{Type: TNil},
		{Type: TContainer, Tokens: []*Token{
			{Type: TContainer, Tokens: []*Token{
				{Type: TAtom, Str: name},
				{Type: TNil},
				{Type: TAtom, Str: "admin"},
				{Type: TAtom, Str: "example.com"},
			}},
		}},
		{Type: TNil},
		{Type: TNil},
		{Type: TNil},
		{Type: TQuoted, Str: "<id>"},
	}}
	records := [][]*Token{{
		{Type: TLiteral, Str: "UID"},
		{Type: TNumber, Num: 1},
		{Type: TLiteral, Str: "ENVELOPE"},
		env,
	}}
	d := &Dialer{}
	emails, err := parseRecords(d, records)
	if err != nil {
		t.Fatalf("parseRecords error: %v", err)
	}
	addr, ok := emails[1].To["admin@example.com"]
	if !ok {
		t.Fatalf("address not parsed")
	}
	if addr != name {
		t.Fatalf("got %q want %q", addr, name)
	}
}
