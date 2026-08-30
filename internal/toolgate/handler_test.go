package toolgate

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGateRequiresLaunch(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "tool-ok")
	}))
	t.Cleanup(upstream.Close)
	store := NewMemory()
	gate, err := NewGate(ToolPgAdmin, upstream.URL, store, false, "https://console.example.com")
	if err != nil {
		t.Fatal(err)
	}

	denied := httptest.NewRecorder()
	gate.ServeHTTP(denied, httptest.NewRequest(http.MethodGet, "/", nil))
	if denied.Code != http.StatusSeeOther || !strings.Contains(denied.Header().Get("Location"), "/system") {
		t.Fatalf("deny = %d %s", denied.Code, denied.Header().Get("Location"))
	}

	raw, err := store.Issue(ToolPgAdmin)
	if err != nil {
		t.Fatal(err)
	}
	launch := httptest.NewRecorder()
	gate.ServeHTTP(launch, httptest.NewRequest(http.MethodGet, LaunchPath+"?ticket="+raw, nil))
	if launch.Code != http.StatusSeeOther {
		t.Fatalf("launch = %d", launch.Code)
	}
	var cookie *http.Cookie
	for _, c := range launch.Result().Cookies() {
		if c.Name == CookieName {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("missing tool cookie")
	}
	if cookie.SameSite != http.SameSiteStrictMode || !cookie.HttpOnly || cookie.Domain != "" {
		t.Fatalf("tool cookie flags = %+v", cookie)
	}

	ok := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookie)
	gate.ServeHTTP(ok, req)
	if ok.Code != http.StatusOK || ok.Body.String() != "tool-ok" {
		t.Fatalf("proxied = %d %s", ok.Code, ok.Body.String())
	}
}

func TestGateHeadDoesNotConsumeTicket(t *testing.T) {
	t.Parallel()
	store := NewMemory()
	gate, err := NewGate(ToolPgAdmin, "http://127.0.0.1:5052", store, false, "https://console.example.com")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := store.Issue(ToolPgAdmin)
	if err != nil {
		t.Fatal(err)
	}
	head := httptest.NewRecorder()
	gate.ServeHTTP(head, httptest.NewRequest(http.MethodHead, LaunchPath+"?ticket="+raw, nil))
	if head.Code != http.StatusMethodNotAllowed {
		t.Fatalf("HEAD launch = %d", head.Code)
	}
	get := httptest.NewRecorder()
	gate.ServeHTTP(get, httptest.NewRequest(http.MethodGet, LaunchPath+"?ticket="+raw, nil))
	if get.Code != http.StatusSeeOther {
		t.Fatalf("GET after HEAD = %d", get.Code)
	}
}
