package redisadmin

import (
	"context"
	"errors"
	"testing"
)

func infoServerVersion(version string) string {
	return "# Server\nredis_version:" + version + "\nredis_mode:standalone\n"
}

func TestCheckServerSeriesMemoryInfoFixtures(t *testing.T) {
	cases := []struct {
		name     string
		version  string
		text     string
		expected string
		wantErr  bool
	}{
		{name: "8.8.2-ok", version: "8.8.2"},
		{name: "8.2.9-ok", version: "8.2.9"},
		{name: "8.8.2-matches-expected", version: "8.8.2", expected: "8.8"},
		{name: "8.2.9-matches-expected", version: "8.2.9", expected: "8.2"},
		{name: "8.10.1-deny", version: "8.10.1", wantErr: true},
		{name: "garbage-deny", text: "this is not INFO output", wantErr: true},
		{name: "expected-mismatch-deny", version: "8.8.2", expected: "8.2", wantErr: true},
		{name: "prerelease-deny", version: "8.8.2-rc1", wantErr: true},
		{name: "empty-deny", text: "", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text := tc.text
			if text == "" && tc.version != "" {
				text = infoServerVersion(tc.version)
			}
			mem := &MemoryClient{InfoText: text}
			got, err := mem.Info(context.Background())
			if err != nil {
				t.Fatalf("Info: %v", err)
			}
			err = checkServerSeries(got, tc.expected)
			if tc.wantErr {
				if !errors.Is(err, ErrUnavailable) {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("checkServerSeries: %v", err)
			}
		})
	}
}

func TestCheckServerSeriesInfoErrDoesNotParse(t *testing.T) {
	mem := &MemoryClient{InfoErr: errors.New("dial tcp 10.0.0.1:6379: connection refused password=canary-secret")}
	text, err := mem.Info(context.Background())
	if err == nil || text != "" {
		t.Fatalf("Info = %q, %v", text, err)
	}
	classified := classifyRedisError(err)
	if !errors.Is(classified, ErrUnavailable) {
		t.Fatalf("classified = %v", classified)
	}
	assertNoRedisCanary(t, classified.Error())
}
