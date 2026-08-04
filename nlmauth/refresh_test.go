package nlmauth

import (
	"crypto/sha1"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestAppOrigin(t *testing.T) {
	const want = "https://notebook.google.com"
	if AppOrigin != want {
		t.Fatalf("AppOrigin = %q, want %q", AppOrigin, want)
	}
}

func TestGenerateSAPISIDHASH(t *testing.T) {
	tests := []struct {
		name      string
		sapisid   string
		timestamp int64
		want      string
	}{
		{
			name:      "Example hash",
			sapisid:   "test-sapisid-cookie",
			timestamp: 1757337921,
			want:      "3fcdcde25419df4771db630a967ca2fa4a83003b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &RefreshClient{
				sapisid: tt.sapisid,
			}

			got := client.generateSAPISIDHASH(tt.timestamp)
			if got != tt.want {
				t.Errorf("generateSAPISIDHASH() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRefreshCredentialsUsesAppOrigin(t *testing.T) {
	const sapisid = "test-sapisid"
	var request *http.Request
	client := &RefreshClient{
		cookies: "SAPISID=" + sapisid,
		sapisid: sapisid,
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			request = req
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     http.StatusText(http.StatusOK),
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		})},
	}

	if err := client.RefreshCredentials(""); err != nil {
		t.Fatalf("RefreshCredentials: %v", err)
	}
	if request == nil {
		t.Fatal("RefreshCredentials sent no request")
	}
	if got := request.Header.Get("Origin"); got != AppOrigin {
		t.Fatalf("Origin = %q, want %q", got, AppOrigin)
	}
	if got := request.Header.Get("Referer"); got != AppOrigin+"/" {
		t.Fatalf("Referer = %q, want %q", got, AppOrigin+"/")
	}

	scheme, value, ok := strings.Cut(request.Header.Get("Authorization"), " ")
	if !ok || scheme != "SAPISIDHASH" {
		t.Fatalf("Authorization = %q, want SAPISIDHASH value", request.Header.Get("Authorization"))
	}
	timestampText, gotHash, ok := strings.Cut(value, "_")
	if !ok {
		t.Fatalf("Authorization value = %q, want timestamp_hash", value)
	}
	timestamp, err := strconv.ParseInt(timestampText, 10, 64)
	if err != nil {
		t.Fatalf("parse Authorization timestamp %q: %v", timestampText, err)
	}
	data := fmt.Sprintf("%d %s %s", timestamp, sapisid, request.Header.Get("Origin"))
	wantHash := fmt.Sprintf("%x", sha1.Sum([]byte(data)))
	if gotHash != wantHash {
		t.Fatalf("SAPISIDHASH = %q, want %q computed from request Origin", gotHash, wantHash)
	}
}

func TestExtractCookieValue(t *testing.T) {
	cookies := "HSID=test-hsid-cookie; SSID=test-ssid-cookie; SAPISID=test-sapisid-cookie; OTHER=value"

	tests := []struct {
		name   string
		cookie string
		want   string
	}{
		{"Extract SAPISID", "SAPISID", "test-sapisid-cookie"},
		{"Extract HSID", "HSID", "test-hsid-cookie"},
		{"Extract SSID", "SSID", "test-ssid-cookie"},
		{"Non-existent cookie", "NOTFOUND", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractCookieValue(cookies, tt.cookie)
			if got != tt.want {
				t.Errorf("extractCookieValue(%s) = %v, want %v", tt.cookie, got, tt.want)
			}
		})
	}
}

func TestParseNotebookLMPageState(t *testing.T) {
	body := []byte(`
<!doctype html>
<html>
<head><script>
window.WIZ_global_data = {"FdrFJe":"-8344731930921376674","cfb2h":"boq_labs-tailwind-frontend_20260406.14_p0"};
</script></head>
<body>{"gsessionid":"LsWt3iCG3ezhLlQau_BO2Gu853yG1uLi0RnZlSwqVfg"}</body>
</html>`)

	got := parseNotebookLMPageState(body)
	if got.GSessionID != "LsWt3iCG3ezhLlQau_BO2Gu853yG1uLi0RnZlSwqVfg" {
		t.Fatalf("GSessionID = %q, want %q", got.GSessionID, "LsWt3iCG3ezhLlQau_BO2Gu853yG1uLi0RnZlSwqVfg")
	}
	if got.SessionID != "-8344731930921376674" {
		t.Fatalf("SessionID = %q, want %q", got.SessionID, "-8344731930921376674")
	}
	if got.BLParam != "boq_labs-tailwind-frontend_20260406.14_p0" {
		t.Fatalf("BLParam = %q, want %q", got.BLParam, "boq_labs-tailwind-frontend_20260406.14_p0")
	}
}

func TestValidateNotebookLMPageURL(t *testing.T) {
	// The page fetch may follow redirects, but only the app origin is valid as
	// the final page from which bootstrap state is parsed.
	tests := []struct {
		name     string
		finalURL string
		wantErr  bool
	}{
		{
			name:     "empty URL allowed",
			finalURL: "",
		},
		{
			name:     "app host accepted",
			finalURL: AppOrigin + "/notebook/notebook-1",
		},
		{
			name:     "account chooser rejected",
			finalURL: "https://accounts.google.com/AccountChooser?continue=" + AppOrigin,
			wantErr:  true,
		},
		{
			name:     "legacy login host rejected as final URL",
			finalURL: "https://notebooklm.google.com/login?continue=" + AppOrigin,
			wantErr:  true,
		},
		{
			name:     "unrelated host rejected",
			finalURL: "https://example.com/",
			wantErr:  true,
		},
		{
			name:     "malformed URL rejected",
			finalURL: "://bad-url",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNotebookLMPageURL(tt.finalURL)
			if tt.wantErr && err == nil {
				t.Fatal("validateNotebookLMPageURL() error = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("validateNotebookLMPageURL() error = %v, want nil", err)
			}
		})
	}
}
