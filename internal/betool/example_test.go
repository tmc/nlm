package betool_test

import (
	"fmt"
	"strings"

	"github.com/tmc/nlm/internal/betool"
)

func Example() {
	options := betool.Options{JSONOutput: true}
	fmt.Println(options.JSONOutput)
	// Output:
	// true
}

func ExampleHelpText() {
	help := betool.HelpText("traffic")
	fmt.Println(strings.Contains(help, "nlm traffic decode-response"))
	// Output: true
}
