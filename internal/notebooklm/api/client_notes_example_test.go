package api_test

import (
	"context"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

func ExampleClient_UpdateNote() {
	client := api.New(api.Credentials{
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
