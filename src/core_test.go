package main

import "testing"

// A made-up token (name "crysticat", expires in 2100). Not a real credential.
const testJWT = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJuYW1lIjoiY3J5c3RpY2F0IiwiZXhwIjo0MTAyNDQ0ODAwfQ.sig"

func TestCleanTokenAndCandidates(t *testing.T) {
	got, err := cleanToken("  '" + testJWT + "' ")
	if err != nil || got != testJWT {
		t.Fatalf("cleanToken = %q, %v", got, err)
	}
	if _, err := cleanToken("hello"); err == nil {
		t.Fatal("expected an error for a non-token")
	}
	c := tokenCandidates(testJWT)
	if len(c) != 2 || c[1] != "triangulet "+testJWT {
		t.Fatalf("candidates = %v", c)
	}
	name, exp, ok := tokenInfo(testJWT)
	if !ok || name != "crysticat" || exp.UTC().Year() != 2100 {
		t.Fatalf("tokenInfo = %q %v %v", name, exp, ok)
	}
}
