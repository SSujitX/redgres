package release

import (
	"strings"
	"testing"
)

func TestCompatibilityPolicyRevisionStartsAtOne(t *testing.T) {
	if CompatibilityPolicyRevision != "1" {
		t.Fatalf("CompatibilityPolicyRevision = %q", CompatibilityPolicyRevision)
	}
}

func TestParseExpectedPostgreSQLMajor(t *testing.T) {
	t.Parallel()
	ok := []struct {
		in   string
		want int
	}{
		{in: "17", want: 17},
		{in: "18", want: 18},
	}
	for _, tc := range ok {
		got, err := ParseExpectedPostgreSQLMajor(tc.in)
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("%q: got %d want %d", tc.in, got, tc.want)
		}
		if !SupportedPostgreSQLMajor(got) {
			t.Fatalf("%q: parsed major is not supported", tc.in)
		}
	}

	denied := []string{
		"", "latest", "latest-tested", "18.6", "17.11", "16", "19", "8.8",
		"18.0", "017", "18-rc1", "18rc1", "v18",
	}
	for _, in := range denied {
		got, err := ParseExpectedPostgreSQLMajor(in)
		if err == nil {
			t.Fatalf("%q: accepted as %d", in, got)
		}
		if got != 0 {
			t.Fatalf("%q: got %d on error", in, got)
		}
	}
}

func TestParseExpectedRedisSeries(t *testing.T) {
	t.Parallel()
	ok := []string{"8.2", "8.8"}
	for _, in := range ok {
		got, err := ParseExpectedRedisSeries(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if got != in {
			t.Fatalf("%q: got %q", in, got)
		}
		if !SupportedRedisSeries(got) {
			t.Fatalf("%q: parsed series is not supported", in)
		}
	}

	denied := []string{
		"", "latest", "latest-tested", "8.8.2", "8.2.9", "8.10", "8.10.1",
		"8.0", "7.2", "8", "8.8.0", "8.2-rc1", "8.8-pre", "v8.8", "08.8",
	}
	for _, in := range denied {
		got, err := ParseExpectedRedisSeries(in)
		if err == nil {
			t.Fatalf("%q: accepted as %q", in, got)
		}
		if got != "" {
			t.Fatalf("%q: got %q on error", in, got)
		}
	}
}

func TestExpectedValuesCannotWidenSupport(t *testing.T) {
	t.Parallel()
	if SupportedPostgreSQLMajor(16) || SupportedPostgreSQLMajor(19) || SupportedPostgreSQLMajor(0) {
		t.Fatal("unsupported PostgreSQL majors must stay unsupported")
	}
	if SupportedRedisSeries("8.10") || SupportedRedisSeries("8.10.1") || SupportedRedisSeries("latest") || SupportedRedisSeries("") {
		t.Fatal("unsupported Redis series must stay unsupported")
	}
	if _, err := ParseExpectedPostgreSQLMajor("19"); err == nil {
		t.Fatal("expected major 19 must not widen support")
	}
	if _, err := ParseExpectedRedisSeries("8.10"); err == nil {
		t.Fatal("expected series 8.10 must not widen support")
	}
}

func TestPostgreSQLMajorFromVersionNum(t *testing.T) {
	t.Parallel()
	cases := []struct {
		num  int
		want int
		ok   bool
	}{
		{num: 170011, want: 17, ok: true},
		{num: 180006, want: 18, ok: true},
		{num: 160000, want: 16, ok: true},
		{num: 0, want: 0, ok: false},
		{num: 90600, want: 9, ok: true},
	}
	for _, tc := range cases {
		got, ok := PostgreSQLMajorFromVersionNum(tc.num)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("version_num %d: got (%d, %v) want (%d, %v)", tc.num, got, ok, tc.want, tc.ok)
		}
	}
}

func TestRedisSeriesFromVersion(t *testing.T) {
	t.Parallel()
	ok := []struct {
		in   string
		want string
	}{
		{in: "8.8.2", want: "8.8"},
		{in: "8.2.9", want: "8.2"},
		{in: "8.10.1", want: "8.10"},
		{in: "8.2.0", want: "8.2"},
	}
	for _, tc := range ok {
		got, err := RedisSeriesFromVersion(tc.in)
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("%q: got %q want %q", tc.in, got, tc.want)
		}
	}

	denied := []string{
		"", "garbage", "8.8", "8.8.2-rc1", "8.8.2-pre", "8.8.2rc1",
		"8.8.2-alpine", "v8.8.2", "8.8.2.1", "latest", "8.10.1-rc1",
	}
	for _, in := range denied {
		got, err := RedisSeriesFromVersion(in)
		if err == nil {
			t.Fatalf("%q: accepted as %q", in, got)
		}
		if got != "" {
			t.Fatalf("%q: got %q on error", in, got)
		}
	}
}

func TestLiveTestPinsAreDigestsNotLatest(t *testing.T) {
	t.Parallel()
	pins := []string{LiveTestPostgres186, LiveTestRedis882, LiveTestPostgres1711, LiveTestRedis829}
	for _, pin := range pins {
		if pin == "" {
			t.Fatal("empty pin")
		}
		if strings.Contains(pin, "latest") {
			t.Fatalf("floating latest in %q", pin)
		}
		if !strings.Contains(pin, "@sha256:") {
			t.Fatalf("missing digest in %q", pin)
		}
		if strings.Contains(strings.ToLower(pin), "pgbouncer") {
			t.Fatalf("PgBouncer image pin is forbidden: %q", pin)
		}
	}
	if PgBouncerProbeSQL != "SHOW VERSION" {
		t.Fatalf("PgBouncerProbeSQL = %q", PgBouncerProbeSQL)
	}
}
