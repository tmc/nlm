package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestChatHTMLDestination(t *testing.T) {
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		out  string
		want string
	}{
		{
			name: "explicit file",
			out:  "conversation.html",
			want: "conversation.html",
		},
		{
			name: "default cache",
			want: filepath.Join(cache, "nlm", "render", "notebook", "conversation.html"),
		},
		{
			name: "stdout",
			out:  "-",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := chatHTMLDestination("notebook", "conversation", test.out)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("destination = %q, want %q", got, test.want)
			}
		})
	}
}

func TestChatHTMLDestinationCreatesNotebookDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	path, err := chatHTMLDestination("notebook", "conversation", "")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", filepath.Dir(path))
	}
}
