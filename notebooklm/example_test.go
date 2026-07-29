package notebooklm_test

import (
	"fmt"

	"github.com/tmc/nlm/notebooklm"
)

func Example() {
	client := notebooklm.New(
		notebooklm.Credentials{AuthToken: "token", Cookies: "cookies"},
		notebooklm.WithDebug(true),
	)
	fmt.Println(client != nil)

	// Output:
	// true
}
