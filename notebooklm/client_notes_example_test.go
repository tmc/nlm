package notebooklm_test

import (
	"context"

	"github.com/tmc/nlm/notebooklm"
)

func ExampleClient_UpdateNote() {
	client := notebooklm.New(notebooklm.Credentials{
		AuthToken: "auth-token",
		Cookies:   "cookies",
	})
	title := "New title"
	_, _ = client.UpdateNote(
		context.Background(),
		"notebook-id",
		"note-id",
		&title,
		nil,
	)
}
