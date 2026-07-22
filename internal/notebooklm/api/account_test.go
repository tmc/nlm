package api

import (
	"testing"
)

func TestParseAccountStatus(t *testing.T) {
	data := []byte(`[[null,[6,500,300,500000,2],[true,null,null,true,[null,null,null,[[2,2,2]]],null,false,null,false],[[1]],[true,1,3,2]]]`)

	got, err := parseAccountStatus(data)
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

func TestParseAccountStatusProtoMatchesLegacy(t *testing.T) {
	data := []byte(`[[null,[6,500,300,500000,2],[true,null,null,true,[null,null,null,[[2,2,2]]],null,false,null,false],[[1]],[true,1,3,2]]]`)
	legacy, err := parseAccountStatus(data)
	if err != nil {
		t.Fatal(err)
	}
	protoStatus, err := parseAccountStatusProto(data)
	if err != nil {
		t.Fatal(err)
	}
	if *protoStatus != *legacy {
		t.Fatalf("proto status = %+v, legacy = %+v", protoStatus, legacy)
	}
}
