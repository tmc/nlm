package auth

import (
	"testing"
)

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
			want:      "61ce8d584412c85e2a0a1adebcd9e2c54bc3223f",
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
