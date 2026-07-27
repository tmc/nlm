package betool_test

import (
	"fmt"

	"github.com/tmc/nlm/internal/betool"
)

func Example() {
	options := betool.Options{JSONOutput: true}
	fmt.Println(options.JSONOutput)
	// Output:
	// true
}
