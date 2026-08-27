package cloudflare

import "testing"

func TestOriginHTTPService(t *testing.T) {
	got, err := OriginHTTPService("127.0.0.1:8790")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://127.0.0.1:8790" {
		t.Fatalf("got %q", got)
	}
	got, err = OriginHTTPService("0.0.0.0:8989")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://127.0.0.1:8989" {
		t.Fatalf("non-loopback listen still maps to loopback origin; got %q", got)
	}
	if _, err := OriginHTTPService("not-an-addr"); err == nil {
		t.Fatal("expected error")
	}
}
