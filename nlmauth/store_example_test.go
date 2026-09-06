package nlmauth_test

import (
	"fmt"
	"os"

	"github.com/tmc/nlm/nlmauth"
)

func ExampleValidateIdentityName() {
	fmt.Println(nlmauth.ValidateIdentityName("work") == nil)
	fmt.Println(nlmauth.ValidateIdentityName("../work") == nil)
	// Output:
	// true
	// false
}

func ExampleFileStore() {
	dir, err := os.MkdirTemp("", "nlmauth-example-")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer os.RemoveAll(dir)
	store := nlmauth.NewFileStore(dir)
	session := nlmauth.Session{Credentials: nlmauth.Credentials{
		AuthToken: "example-token", Cookies: "example-cookies",
	}}
	if err := store.Set("work", session); err != nil {
		fmt.Println(err)
		return
	}
	names, err := store.List()
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(names)
	stored, err := store.Get("work")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(stored == session)
	if err := store.Delete("work"); err != nil {
		fmt.Println(err)
		return
	}
	names, err = store.List()
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(len(names))
	// Output:
	// [work]
	// true
	// 0
}
