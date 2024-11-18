package beprotojson

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	// TODO: separate from anything nlm related
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

func TestUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		want    proto.Message
		wantErr bool
	}{
		{
			name: "basic project",
			json: `["project1", [], "id1", "📚"]`,
			want: &pb.Project{
				Title:     "project1",
				ProjectId: "id1",
				Emoji:     "📚",
			},
		},
		{
			name: "project with sources",
			json: `["project2", [[["source1"], "Source One"]], "id2", "📚"]`,
			want: &pb.Project{
				Title: "project2",
				Sources: []*pb.Source{
					{
						SourceId: &pb.SourceId{SourceId: "source1"},
						Title:    "Source One",
					},
				},
				ProjectId: "id2",
				Emoji:     "📚",
			},
		},
		{
			name: "project with youtube sources",
			json: `[
        "Untitled notebook",
        [
            [
                [["39ed97de-7b93-4e08-8d9b-b86d5a58b35a"]],
                "Building with Anthropic Claude: Prompt Workshop with Zack Witten",
                [null, 15108, [1728034802, 578385000],
                 ["0319adc7-1458-4555-a813-17aff0f72938", [1728034801, 818692000]],
                 9,
                 ["https://www.youtube.com/watch?v=hkhDdcM5V94", "hkhDdcM5V94", "AI Engineer"]],
                [null, 2]
            ]
        ],
        "ec266e3d-cb7a-4c6d-a34a-f108a55faf52",
        "🕵️",
        null,
        [
            1,
            false,
            true,
            null,
            null,
            [1731910459, 665561000],
            1,
            false,
            [1731827837, 76688000]
        ]
    ]`,
			want: &pb.Project{
				Title:     "Untitled notebook",
				ProjectId: "ec266e3d-cb7a-4c6d-a34a-f108a55faf52",
				Sources: []*pb.Source{
					{
						SourceId: &pb.SourceId{SourceId: "39ed97de-7b93-4e08-8d9b-b86d5a58b35a"},
						Title:    "Building with Anthropic Claude: Prompt Workshop with Zack Witten",
						Metadata: &pb.SourceMetadata{
							LastUpdateTimeSeconds: &wrapperspb.Int32Value{Value: 15108},
							LastModifiedTime: &timestamppb.Timestamp{
								Seconds: 1728034802,
								Nanos:   578385000,
							},
							SourceType: pb.SourceType_SOURCE_TYPE_YOUTUBE_VIDEO,
							MetadataType: &pb.SourceMetadata_Youtube{
								Youtube: &pb.YoutubeSourceMetadata{
									YoutubeUrl: "https://www.youtube.com/watch?v=hkhDdcM5V94",
									VideoId:    "hkhDdcM5V94",
								},
							},
						},
						Settings: &pb.SourceSettings{
							Status: pb.SourceSettings_SOURCE_STATUS_DISABLED,
						},
					},
				},
				Emoji: "🕵️",
				Metadata: &pb.ProjectMetadata{
					UserRole: 1,
					Type:     1,
					CreateTime: &timestamppb.Timestamp{
						Seconds: 1731827837,
						Nanos:   76688000,
					},
					ModifiedTime: &timestamppb.Timestamp{
						Seconds: 1731910459,
						Nanos:   665561000,
					},
				},
			},
		},
		{
			name:    "invalid json",
			json:    `not json`,
			want:    &pb.Project{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := &pb.Project{}
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

func TestUnmarshalOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    UnmarshalOptions
		json    string
		want    *pb.Project // Changed to concrete type
		wantErr bool
	}{
		{
			name: "discard unknown fields",
			opts: UnmarshalOptions{DiscardUnknown: true},
			json: `["project1", [], "id1", "📚", null, [1, false, true, null, null, [1731910459, 665561000], 1, false, [1731827837, 76688000]]]`,
			want: &pb.Project{
				Title:     "project1",
				ProjectId: "id1",
				Emoji:     "📚",
				Metadata: &pb.ProjectMetadata{
					UserRole: 1,
					Type:     1,
					CreateTime: &timestamppb.Timestamp{
						Seconds: 1731827837,
						Nanos:   76688000,
					},
					ModifiedTime: &timestamppb.Timestamp{
						Seconds: 1731910459,
						Nanos:   665561000,
					},
				},
			},
		},
		{
			name:    "fail on unknown fields",
			opts:    UnmarshalOptions{DiscardUnknown: false},
			json:    `["project1", [], "My Project", "📚", "extra"]`,
			want:    &pb.Project{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := &pb.Project{} // Create new instance directly
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

// TestRoundTrip tests marshaling and unmarshaling
func TestRoundTrip(t *testing.T) {
	t.Skip("Marshal not implemented yet")

	tests := []struct {
		name string
		msg  proto.Message
	}{
		{
			name: "basic project",
			msg: &pb.Project{
				ProjectId: "project1",
				Title:     "My Project",
				Emoji:     "📚",
			},
		},
		// Add more test cases here
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
				t.Fatalf("Unmarshal() error = %v", err)
			}

			if diff := cmp.Diff(tt.msg, got, protocmp.Transform()); diff != "" {
				t.Errorf("Round trip diff (-want +got):\n%s", diff)
			}
		})
	}
}
