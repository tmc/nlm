package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

// TestInteractiveLogin exercises the actual browser against a local sign-in
// page. Opt in on a desktop with Brave or Chrome installed:
//
//	NLM_TEST_BROWSER=1 go test ./internal/auth -run TestInteractiveLogin
func TestInteractiveLogin(t *testing.T) {
	if os.Getenv("NLM_TEST_BROWSER") != "1" {
		t.Skip("set NLM_TEST_BROWSER=1 to test with a visible browser")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	const token = "local-test-token-long-enough-for-validation"
	webdriver := make(chan string, 1)
	var signins atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			if cookie, err := r.Cookie("SID"); err == nil && cookie.Value == strings.Repeat("x", 60) {
				http.Redirect(w, r, "/notebook", http.StatusFound)
			} else {
				http.Redirect(w, r, "/signin", http.StatusFound)
			}
		case "/signin":
			signins.Add(1)
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, `<body><input type="email"><script>
window.WIZ_global_data = {SNlM0e: %q};
fetch('/webdriver?value=' + navigator.webdriver);
setTimeout(() => location.href = '/notebook', 8000);
</script></body>`, token)
		case "/webdriver":
			select {
			case webdriver <- r.URL.Query().Get("value"):
			default:
			}
		case "/notebook":
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: strings.Repeat("x", 60), Path: "/", MaxAge: 3600})
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, `<body>Signed in<script>window.WIZ_global_data = {SNlM0e: %q};</script></body>`, token)
		}
	}))
	defer server.Close()

	profileDir := t.TempDir()
	for range 2 {
		data, err := New(false).GetAuthData(WithTargetURL(server.URL), func(o *Options) {
			o.InteractiveLogin = true
			o.UserDataDir = profileDir
		})
		if err != nil {
			t.Fatal(err)
		}
		if data.Token != token || !strings.Contains(data.Cookies, "SID="+strings.Repeat("x", 60)) {
			t.Fatal("browser did not capture the completed local session")
		}
	}
	if got := signins.Load(); got != 1 {
		t.Errorf("sign-in visits = %d, want 1 across two browser launches", got)
	}
	select {
	case value := <-webdriver:
		if value != "false" {
			t.Errorf("navigator.webdriver = %q, want false for interactive sign-in", value)
		}
	default:
		t.Error("browser did not visit the sign-in page")
	}
}
