package beprotojson

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestUnmarshalBasicTypes tests beprotojson with standard protobuf well-known types
// This verifies the core functionality without depending on nlm-specific types
func TestUnmarshalBasicTypes(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		want    proto.Message
		wantErr bool
	}{
		{
			name: "timestamp",
			json: `[1728034802, 578385000]`,
			want: &timestamppb.Timestamp{
				Seconds: 1728034802,
				Nanos:   578385000,
			},
		},
		{
			name: "int32_wrapper",
			json: `[15108]`,
			want: &wrapperspb.Int32Value{
				Value: 15108,
			},
		},
		{
			name: "string_wrapper",
			json: `["test value"]`,
			want: &wrapperspb.StringValue{
				Value: "test value",
			},
		},
		{
			name: "bool_wrapper_true",
			json: `[true]`,
			want: &wrapperspb.BoolValue{
				Value: true,
			},
		},
		{
			name: "bool_wrapper_false",
			json: `[false]`,
			want: &wrapperspb.BoolValue{
				Value: false,
			},
		},
		{
			name:    "invalid json",
			json:    `not json`,
			want:    &structpb.Struct{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := proto.Clone(tt.want)
			proto.Reset(got)

			err := Unmarshal([]byte(tt.json), got)
			if (err != nil) != tt.wantErr {
				t.Errorf("Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			if diff := cmp.Diff(tt.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("Unmarshal() diff (-want +got):\n%s", diff)
			}
		})
	}
}

// TestUnmarshalOptionsBasic tests UnmarshalOptions with simple types
func TestUnmarshalOptionsBasic(t *testing.T) {
	tests := []struct {
		name    string
		opts    UnmarshalOptions
		json    string
		want    proto.Message
		wantErr bool
	}{
		{
			name: "discard unknown fields - timestamp with extra",
			opts: UnmarshalOptions{DiscardUnknown: true},
			json: `[1728034802, 578385000, "extra", 123]`,
			want: &timestamppb.Timestamp{
				Seconds: 1728034802,
				Nanos:   578385000,
			},
		},
		{
			name:    "fail on unknown fields",
			opts:    UnmarshalOptions{DiscardUnknown: false},
			json:    `[1728034802, 578385000, "extra"]`,
			want:    &timestamppb.Timestamp{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := proto.Clone(tt.want)
			proto.Reset(got)

			err := tt.opts.Unmarshal([]byte(tt.json), got)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalOptions.Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			if diff := cmp.Diff(tt.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("UnmarshalOptions.Unmarshal() diff (-want +got):\n%s", diff)
			}
		})
	}
}
