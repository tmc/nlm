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

// TestMarshalBasicTypes tests Marshal with standard protobuf well-known types
func TestMarshalBasicTypes(t *testing.T) {
	tests := []struct {
		name string
		msg  proto.Message
		want string
	}{
		{
			name: "nil message",
			msg:  (*timestamppb.Timestamp)(nil),
			want: `null`,
		},
		{
			name: "timestamp",
			msg: &timestamppb.Timestamp{
				Seconds: 1728034802,
				Nanos:   578385000,
			},
			want: `[1728034802,578385000]`,
		},
		{
			name: "int32 wrapper",
			msg:  &wrapperspb.Int32Value{Value: 42},
			want: `[42]`,
		},
		{
			name: "string wrapper",
			msg:  &wrapperspb.StringValue{Value: "hello"},
			want: `["hello"]`,
		},
		{
			name: "bool wrapper true",
			msg:  &wrapperspb.BoolValue{Value: true},
			want: `[1]`,
		},
		{
			name: "bool wrapper false",
			msg:  &wrapperspb.BoolValue{Value: false},
			// In proto3, false is the default so Has() returns false; serializes as null
			want: `[null]`,
		},
		{
			name: "zero timestamp",
			msg:  &timestamppb.Timestamp{},
			want: `[null,null]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Marshal(tt.msg)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("Marshal() = %s, want %s", got, tt.want)
			}
		})
	}
}

// TestMarshalRoundTripBasic tests Marshal → Unmarshal round-trip with well-known types
func TestMarshalRoundTripBasic(t *testing.T) {
	tests := []struct {
		name string
		msg  proto.Message
	}{
		{
			name: "timestamp",
			msg: &timestamppb.Timestamp{
				Seconds: 1728034802,
				Nanos:   578385000,
			},
		},
		{
			name: "int32 wrapper",
			msg:  &wrapperspb.Int32Value{Value: 42},
		},
		{
			name: "string wrapper",
			msg:  &wrapperspb.StringValue{Value: "hello world"},
		},
		{
			name: "bool wrapper true",
			msg:  &wrapperspb.BoolValue{Value: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := Marshal(tt.msg)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}

			got := proto.Clone(tt.msg)
			proto.Reset(got)

			if err := Unmarshal(data, got); err != nil {
				t.Fatalf("Unmarshal(%s) error = %v", string(data), err)
			}

			if diff := cmp.Diff(tt.msg, got, protocmp.Transform()); diff != "" {
				t.Errorf("Round trip diff (-want +got):\n%s", diff)
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
