package rpc

import "testing"

func TestExtractBatchExecuteParams(t *testing.T) {
	t.Run("extracts bl fsid hl from HTML", func(t *testing.T) {
		html := `
<html>
<head>
<script>
var cfg = {"bl":"boq_labs-tailwind-frontend_20260129.10_p0","hl":"en"};
var params = {"f.sid":"3453249741565402435"};
</script>
</head>
</html>`
		bl, fsid, hl := extractBatchExecuteParams(html)
		if bl != "boq_labs-tailwind-frontend_20260129.10_p0" {
			t.Fatalf("bl = %q, want %q", bl, "boq_labs-tailwind-frontend_20260129.10_p0")
		}
		if fsid != "3453249741565402435" {
			t.Fatalf("fsid = %q, want %q", fsid, "3453249741565402435")
		}
		if hl != "en" {
			t.Fatalf("hl = %q, want %q", hl, "en")
		}
	})

	t.Run("supports single quotes and negative fsid", func(t *testing.T) {
		html := `
<script>
var cfg = {'bl':'boq_labs-tailwind-frontend_20250129.00_p0','hl':'en-US'};
var params = {'f.sid':'-7121977511756781186'};
</script>`
		bl, fsid, hl := extractBatchExecuteParams(html)
		if bl != "boq_labs-tailwind-frontend_20250129.00_p0" {
			t.Fatalf("bl = %q, want %q", bl, "boq_labs-tailwind-frontend_20250129.00_p0")
		}
		if fsid != "-7121977511756781186" {
			t.Fatalf("fsid = %q, want %q", fsid, "-7121977511756781186")
		}
		if hl != "en-US" {
			t.Fatalf("hl = %q, want %q", hl, "en-US")
		}
	})

	t.Run("missing values return empty strings", func(t *testing.T) {
		html := `<script>var cfg = {"bl":"boq_labs-tailwind-frontend_20260127.09_p1"};</script>`
		bl, fsid, hl := extractBatchExecuteParams(html)
		if bl != "boq_labs-tailwind-frontend_20260127.09_p1" {
			t.Fatalf("bl = %q, want %q", bl, "boq_labs-tailwind-frontend_20260127.09_p1")
		}
		if fsid != "" {
			t.Fatalf("fsid = %q, want empty", fsid)
		}
		if hl != "" {
			t.Fatalf("hl = %q, want empty", hl)
		}
	})
}

func TestNewClient_AuthUserConfig(t *testing.T) {
	t.Setenv("NLM_AUTHUSER", "1")
	t.Setenv("NLM_BL", "boq_labs-tailwind-frontend_20260129.10_p0")
	t.Setenv("NLM_F_SID", "3453249741565402435")
	t.Setenv("NLM_HL", "en")

	client := New("token", "cookies")
	if client.Config.URLParams["authuser"] != "1" {
		t.Fatalf("authuser url param = %q, want %q", client.Config.URLParams["authuser"], "1")
	}
	if client.Config.Headers["x-goog-authuser"] != "1" {
		t.Fatalf("x-goog-authuser header = %q, want %q", client.Config.Headers["x-goog-authuser"], "1")
	}
}

func TestNewClient_EnvOverrides(t *testing.T) {
	t.Setenv("NLM_AUTHUSER", "0")
	t.Setenv("NLM_BL", "boq_labs-tailwind-frontend_20990101.00_p0")
	t.Setenv("NLM_F_SID", "-123")
	t.Setenv("NLM_HL", "fr")

	client := New("token", "cookies")
	if client.Config.URLParams["bl"] != "boq_labs-tailwind-frontend_20990101.00_p0" {
		t.Fatalf("bl url param = %q, want %q", client.Config.URLParams["bl"], "boq_labs-tailwind-frontend_20990101.00_p0")
	}
	if client.Config.URLParams["f.sid"] != "-123" {
		t.Fatalf("f.sid url param = %q, want %q", client.Config.URLParams["f.sid"], "-123")
	}
	if client.Config.URLParams["hl"] != "fr" {
		t.Fatalf("hl url param = %q, want %q", client.Config.URLParams["hl"], "fr")
	}
}
