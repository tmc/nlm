package method

import (
	"encoding/json"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

func TestEncodeGenerateAccessTokenArgs(t *testing.T) {
	got, err := json.Marshal(EncodeGenerateAccessTokenArgs(&pb.GenerateAccessTokenRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `[]` {
		t.Fatalf("EncodeGenerateAccessTokenArgs = %s, want []", got)
	}
}
