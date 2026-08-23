package postgresadmin

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

func TestEncodeCell(t *testing.T) {
	got, err := encodeCell("boolean", true)
	if err != nil || got != true {
		t.Fatalf("bool = %v %v", got, err)
	}
	got, err = encodeCell("integer", int64(7))
	if err != nil || got != int64(7) {
		t.Fatalf("int = %v %v", got, err)
	}
	got, err = encodeCell("double precision", math.Inf(1))
	if err != nil || got != "+Inf" {
		t.Fatalf("inf = %v %v", got, err)
	}
	got, err = encodeCell("numeric", "1.50")
	if err != nil || got != "1.50" {
		t.Fatalf("numeric = %v %v", got, err)
	}
	got, err = encodeCell("bytea", []byte{0xDE, 0xAD})
	if err != nil || got != `\xdead` {
		t.Fatalf("bytea = %v %v", got, err)
	}
	ts := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	got, err = encodeCell("timestamptz", ts)
	if err != nil || got != "2026-08-23T12:00:00Z" {
		t.Fatalf("time = %v %v", got, err)
	}
	raw := json.RawMessage(`{"a":1}`)
	got, err = encodeCell("jsonb", raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := json.Marshal(map[string]any{"v": got}); err != nil {
		t.Fatal(err)
	}
	if _, err := encodeCell("text", []byte{0xff}); err == nil {
		t.Fatal("invalid utf-8")
	}
}

func TestRowSearchClause(t *testing.T) {
	where, args := rowSearchClause([]columnMeta{{Name: "name", DataType: "text"}, {Name: "id", DataType: "integer"}}, "ab")
	if where == "" || len(args) != 1 || args[0] != "%ab%" {
		t.Fatalf("%q %#v", where, args)
	}
	if _, args := rowSearchClause([]columnMeta{{Name: "id", DataType: "integer"}}, "ab"); args != nil {
		t.Fatalf("no text columns: %#v", args)
	}
}
