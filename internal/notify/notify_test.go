package notify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConfig_Enabled(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"webhook set", Config{WebhookURL: "https://discord.com/api/webhooks/1/abc"}, true},
		{"empty", Config{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.Enabled(); got != tc.want {
				t.Errorf("Enabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNotify_PostsContentToWebhook(t *testing.T) {
	var gotBody map[string]string
	var gotContentType string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	restore := swapDefaultHTTPClientForTest(t, srv.Client())
	defer restore()

	err := Notify(Config{WebhookURL: srv.URL}, "title", "message")
	if err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	if gotBody["content"] != "title\nmessage" {
		t.Errorf("content = %q, want %q", gotBody["content"], "title\nmessage")
	}
}

func TestNotify_DisabledConfig_DoesNothing(t *testing.T) {
	called := false
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	restore := swapDefaultHTTPClientForTest(t, srv.Client())
	defer restore()

	if err := Notify(Config{}, "title", "message"); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}
	if called {
		t.Error("Notify should not call the webhook when the config is disabled")
	}
}

func TestNotify_NonSuccessStatus_ReturnsError(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message": "Unauthorized", "code": 50027}`))
	}))
	defer srv.Close()
	restore := swapDefaultHTTPClientForTest(t, srv.Client())
	defer restore()

	err := Notify(Config{WebhookURL: srv.URL}, "title", "message")
	if err == nil {
		t.Fatal("Notify() = nil, want error")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error %q does not mention status 401", err.Error())
	}
	if !strings.Contains(err.Error(), "Unauthorized") {
		t.Errorf("error %q does not include response body", err.Error())
	}
}

func TestNotify_NetworkFailure_ReturnsError(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	client := srv.Client()
	unreachable := srv.URL
	srv.Close() // closed immediately: the address now refuses connections

	restore := swapDefaultHTTPClientForTest(t, client)
	defer restore()

	err := Notify(Config{WebhookURL: unreachable}, "title", "message")
	if err == nil {
		t.Fatal("Notify() = nil, want error")
	}
	if !strings.Contains(err.Error(), "discord notify") {
		t.Errorf("error %q missing discord notify context", err.Error())
	}
}

func TestNotify_NetworkFailure_DoesNotLeakWebhookURL(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	client := srv.Client()
	secretURL := srv.URL + "/webhooks/123/super-secret-token"
	srv.Close() // closed immediately: the address now refuses connections

	restore := swapDefaultHTTPClientForTest(t, client)
	defer restore()

	err := Notify(Config{WebhookURL: secretURL}, "title", "message")
	if err == nil {
		t.Fatal("Notify() = nil, want error")
	}
	if strings.Contains(err.Error(), "super-secret-token") {
		t.Errorf("error %q leaks the webhook URL/token", err.Error())
	}
}

func TestNotify_RejectsNonHTTPSWebhookURL(t *testing.T) {
	err := Notify(Config{WebhookURL: "http://discord.com/api/webhooks/1/abc"}, "title", "message")
	if err == nil {
		t.Fatal("Notify() = nil, want error for a non-HTTPS webhook URL")
	}
	if !strings.Contains(err.Error(), "https") {
		t.Errorf("error %q does not explain the https requirement", err.Error())
	}
}

// swapDefaultHTTPClientForTest points the process-wide http.DefaultClient
// (which Notify sends through) at client for the duration of the test.
func swapDefaultHTTPClientForTest(t *testing.T, client *http.Client) (restore func()) {
	t.Helper()
	orig := http.DefaultClient
	http.DefaultClient = client
	return func() { http.DefaultClient = orig }
}
