package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"rsc.io/script"
	"rsc.io/script/scripttest"
)

func TestChatTemplateScript(t *testing.T) {
	binary, err := filepath.Abs("nlm_test")
	if err != nil {
		t.Fatal(err)
	}
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "NLM_TEST_BINARY=" + binary}
	scripttest.Test(t, context.Background(), script.NewEngine(), env, "testdata/chat_template.txtar")
}
