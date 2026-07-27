package richrender_test

import (
	"fmt"
	"strings"

	"github.com/tmc/nlm/internal/richrender"
)

func Example() {
	doc := richrender.NoteDocument{
		Title: "Plan",
		Flat:  "Ship it.",
	}
	var out strings.Builder
	if err := richrender.RenderNoteMarkdown(&out, doc); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Print(out.String())
	// Output:
	// # Plan
	//
	// Ship it.
}
