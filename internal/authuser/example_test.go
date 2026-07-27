package authuser_test

import (
	"fmt"

	"github.com/tmc/nlm/internal/authuser"
)

func ExampleNormalize() {
	fmt.Println(authuser.Normalize("0"))
	fmt.Println(authuser.Normalize("2"))
	// Output:
	//
	// 2
}
