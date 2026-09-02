package auth

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestChromeContextsUseWrapper rejects direct calls to chromedp.NewContext.
// Contexts built outside newChromeContext log through the standard logger and
// spill unprefixed library output onto the user's terminal.
func TestChromeContextsUseWrapper(t *testing.T) {
	forEachCall(t, "chromedp", "NewContext", func(fn string, pos token.Position) {
		if fn != "newChromeContext" {
			t.Errorf("%s: chromedp.NewContext called directly in %s; use newChromeContext", pos, fn)
		}
	})
}

// TestNewChromeContextBindsLoggers verifies the wrapper binds both chromedp
// log sinks, not just one: chromedp's errf falls back to logf only when errf
// is unset, and it is errf that carries the event-decoding noise.
func TestNewChromeContextBindsLoggers(t *testing.T) {
	for _, option := range []string{"WithLogf", "WithErrorf"} {
		found := false
		forEachCall(t, "chromedp", option, func(fn string, _ token.Position) {
			if fn == "newChromeContext" {
				found = true
			}
		})
		if !found {
			t.Errorf("newChromeContext does not pass chromedp.%s", option)
		}
	}
}

// TestChromeLogfDropsNoiseUnlessDebug covers the two library-logging modes:
// silent by default, prefixed diagnostics under debug.
func TestChromeLogfDropsNoiseUnlessDebug(t *testing.T) {
	got := captureStderr(t, func() {
		chromeLogf(false)("could not unmarshal event: %s", "boom")
	})
	if got != "" {
		t.Errorf("chromeLogf(false) wrote %q; want nothing", got)
	}

	got = captureStderr(t, func() {
		chromeLogf(true)("could not unmarshal event: %s", "boom")
	})
	want := "nlm: debug: could not unmarshal event: boom\n"
	if got != want {
		t.Errorf("chromeLogf(true) wrote %q, want %q", got, want)
	}
}

// TestStatusfPrefix verifies status lines are prefixed and newline-terminated
// exactly once, whether or not the caller supplied a trailing newline.
func TestStatusfPrefix(t *testing.T) {
	got := captureStderr(t, func() {
		statusf("waiting for %s...\n", "Chrome")
	})
	if want := "nlm: waiting for Chrome...\n"; got != want {
		t.Errorf("statusf wrote %q, want %q", got, want)
	}
}

// forEachCall reports every call to pkg.name in the package's non-test
// sources, along with the enclosing function's name.
func forEachCall(t *testing.T, pkg, name string, report func(fn string, pos token.Position)) {
	t.Helper()

	// Every .go file is parsed, build tags included, so a per-platform file
	// cannot smuggle in a direct call that this host never compiles.
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob package: %v", err)
	}
	fset := token.NewFileSet()
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != name {
					return true
				}
				if id, ok := sel.X.(*ast.Ident); !ok || id.Name != pkg {
					return true
				}
				report(fn.Name.Name, fset.Position(call.Pos()))
				return true
			})
		}
	}
}

// captureStderr runs f with os.Stderr redirected and returns what it wrote.
func captureStderr(t *testing.T, f func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	saved := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = saved }()

	f()
	w.Close()

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stderr: %v", err)
	}
	return string(out)
}
