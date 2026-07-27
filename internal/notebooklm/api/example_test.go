package api

import "fmt"

func ExampleNew() {
	client := New(
		Credentials{AuthToken: "token", Cookies: "cookies"},
		WithDebug(true),
	)
	fmt.Println(client != nil)

	// Output:
	// true
}
