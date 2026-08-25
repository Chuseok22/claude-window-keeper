package notify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConfig_Enabled(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"both set", Config{BotToken: "t", ChatID: "c"}, true},
		{"missing token", Config{ChatID: "c"}, false},
		{"missing chat id", Config{BotToken: "t"}, false},
		{"neither", Config{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.Enabled(); got != tc.want {
				t.Errorf("Enabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNotify_PostsToTelegramAPI(t *testing.T) {
	var gotPath string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	restore := swapTelegramBaseURLForTest(t, srv.URL)
	defer restore()

	Notify(Config{BotToken: "TESTTOKEN", ChatID: "12345"}, "title", "message")

	wantPath := "/botTESTTOKEN/sendMessage"
	if gotPath != wantPath {
		t.Errorf("path = %q, want %q", gotPath, wantPath)
	}
	if gotBody["chat_id"] != "12345" {
		t.Errorf("chat_id = %q, want %q", gotBody["chat_id"], "12345")
	}
	if gotBody["text"] != "title\nmessage" {
		t.Errorf("text = %q, want %q", gotBody["text"], "title\nmessage")
	}
}

func TestNotify_DisabledConfig_DoesNothing(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	restore := swapTelegramBaseURLForTest(t, srv.URL)
	defer restore()

	Notify(Config{}, "title", "message")

	if called {
		t.Error("Notify should not call the API when the config is disabled")
	}
}

func swapTelegramBaseURLForTest(t *testing.T, url string) (restore func()) {
	t.Helper()
	orig := telegramBaseURL
	telegramBaseURL = url
	return func() { telegramBaseURL = orig }
}
