package api

import (
	"encoding/json"
	"testing"

	genmethod "github.com/tmc/nlm/gen/method"
)

func TestAccountRequestEncoderMatchesCorpus(t *testing.T) {
	got := genmethod.EncodeGetOrCreateAccountArgs(accountRequest())
	want := []interface{}{[]interface{}{2, nil, []interface{}{1}, []interface{}{1, nil, nil, nil, nil, nil, nil, nil, nil, nil, []interface{}{1, 3}}}}
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("encoded account request = %#v, want %#v", got, want)
	}
}

func TestParseAccountStatusProto(t *testing.T) {
	data := []byte(`[[null,[6,500,300,500000,2],[true,null,null,true,[null,null,null,[[2,2,2]]],null,false,null,false],[[1]],[true,1,3,2]]]`)

	got, err := parseAccountStatusProto(data)
	if err != nil {
		t.Fatalf("parseAccountStatus() error = %v", err)
	}

	tests := []struct {
		name string
		got  int
		want int
	}{
		{"NotebookLimit", got.NotebookLimit, 500},
		{"SourceLimit", got.SourceLimit, 300},
		{"UploadLimit", got.UploadLimit, 500000},
		{"Tier", got.Tier, 2},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.want)
		}
	}
}
