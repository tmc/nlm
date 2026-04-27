package api

import (
	"reflect"
	"testing"
)

func TestParseLabelsResponse(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []Label
	}{
		{
			name: "empty array",
			in:   `[]`,
			want: []Label{},
		},
		{
			name: "wrapped empty",
			in:   `[[]]`,
			want: []Label{},
		},
		{
			name: "single label flat",
			in:   `[["Generated Code",[["src-1"],["src-2"]],"label-uuid-1",""]]`,
			want: []Label{{
				Name:      "Generated Code",
				LabelID:   "label-uuid-1",
				SourceIDs: []string{"src-1", "src-2"},
			}},
		},
		{
			name: "multiple labels wrapped",
			in:   `[[["Testing",[["src-3"]],"label-2",""],["RPC and Networking",[["src-4"],["src-5"]],"label-3",""]]]`,
			want: []Label{
				{Name: "Testing", LabelID: "label-2", SourceIDs: []string{"src-3"}},
				{Name: "RPC and Networking", LabelID: "label-3", SourceIDs: []string{"src-4", "src-5"}},
			},
		},
		{
			name: "skip rows with no id and no name",
			in:   `[["",[],"",""], ["Real",[["src-x"]],"id-real",""]]`,
			want: []Label{{Name: "Real", LabelID: "id-real", SourceIDs: []string{"src-x"}}},
		},
		{
			// agX4Bc returns [null, [[row, ...]]]; parser must skip the
			// leading null status slot.
			name: "agX4Bc null-prefix wrapped",
			in:   `[null, [["Generated Code",[["src-1"]],"label-uuid-1",""]]]`,
			want: []Label{{Name: "Generated Code", LabelID: "label-uuid-1", SourceIDs: []string{"src-1"}}},
		},
		{
			name: "agX4Bc null-prefix empty",
			in:   `[null, []]`,
			want: []Label{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLabelsResponse([]byte(tt.in))
			if err != nil {
				t.Fatalf("parseLabelsResponse: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v\nwant %#v", got, tt.want)
			}
		})
	}
}
