package storage

import "testing"

func TestSQLLiteralHelpers(t *testing.T) {
	if got := sqlString("Bob's event"); got != "'Bob''s event'" {
		t.Fatalf("unexpected sql string: %s", got)
	}
	if got := sqlNullableString(""); got != "NULL" {
		t.Fatalf("unexpected null string: %s", got)
	}
	v := 42
	if got := sqlNullableInt(&v); got != "42" {
		t.Fatalf("unexpected int: %s", got)
	}
	if got := sqlBool(true); got != "TRUE" {
		t.Fatalf("unexpected bool: %s", got)
	}
}

func TestFirstN(t *testing.T) {
	if got := firstN("abcdef", 3); got != "abc" {
		t.Fatalf("unexpected firstN result: %s", got)
	}
	if got := firstN("ab", 3); got != "ab" {
		t.Fatalf("unexpected firstN short result: %s", got)
	}
}
