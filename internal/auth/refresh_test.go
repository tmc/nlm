package auth

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
	if appOrigin != want {
		t.Fatalf("appOrigin = %q, want %q", appOrigin, want)
	}
	if got := defaultBrowserAuthOptions().TargetURL; got != appOrigin {
		t.Fatalf("default TargetURL = %q, want appOrigin %q", got, appOrigin)
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
			sapisid:   "ehxTF4-jACAOIp6k/Ax2l7oysalHiZneAB",
			timestamp: 1757337921,
			want:      "30fc826e39451a7a4cd75a0621013cc4afe1d6de",
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
	if got := request.Header.Get("Origin"); got != appOrigin {
		t.Fatalf("Origin = %q, want %q", got, appOrigin)
	}
	if got := request.Header.Get("Referer"); got != appOrigin+"/" {
		t.Fatalf("Referer = %q, want %q", got, appOrigin+"/")
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
	cookies := "HSID=ALqRa_fZCerZVJzYF; SSID=Asj5yorYk-Zr-smiU; SAPISID=ehxTF4-jACAOIp6k/Ax2l7oysalHiZneAB; OTHER=value"

	tests := []struct {
		name   string
		cookie string
		want   string
	}{
		{"Extract SAPISID", "SAPISID", "ehxTF4-jACAOIp6k/Ax2l7oysalHiZneAB"},
		{"Extract HSID", "HSID", "ALqRa_fZCerZVJzYF"},
		{"Extract SSID", "SSID", "Asj5yorYk-Zr-smiU"},
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
			finalURL: appOrigin + "/notebook/notebook-1",
		},
		{
			name:     "account chooser rejected",
			finalURL: "https://accounts.google.com/AccountChooser?continue=" + appOrigin,
			wantErr:  true,
		},
		{
			name:     "legacy login host rejected as final URL",
			finalURL: "https://notebooklm.google.com/login?continue=" + appOrigin,
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
