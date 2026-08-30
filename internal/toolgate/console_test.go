package toolgate

import "testing"

func TestPreferredConsoleURLRejectsBootstrap(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		base       string
		persisted  string
		want       string
	}{
		{name: "persisted https wins", base: "http://203.0.113.10:8989", persisted: "https://console.redgres.com", want: "https://console.redgres.com"},
		{name: "https base", base: "https://console.example.com", persisted: "", want: "https://console.example.com"},
		{name: "loopback http ok", base: "http://127.0.0.1:8790", persisted: "", want: "http://127.0.0.1:8790"},
		{name: "bootstrap port rejected", base: "http://203.0.113.10:8989", persisted: "", want: ""},
		{name: "https on 8989 rejected", base: "https://203.0.113.10:8989", persisted: "", want: ""},
		{name: "public http ip rejected", base: "http://45.76.250.202:8790", persisted: "", want: ""},
		{name: "empty persisted ignored", base: "http://127.0.0.1:8989", persisted: "  ", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := PreferredConsoleURL(tc.base, tc.persisted)
			if got != tc.want {
				t.Fatalf("PreferredConsoleURL(%q, %q) = %q, want %q", tc.base, tc.persisted, got, tc.want)
			}
		})
	}
}

func TestOriginSetGet(t *testing.T) {
	t.Parallel()
	o := NewOrigin("http://203.0.113.10:8989")
	o.Set(PreferredConsoleURL("http://203.0.113.10:8989", "https://console.example.com"))
	if got := o.Get(); got != "https://console.example.com" {
		t.Fatalf("Get = %q", got)
	}
}
