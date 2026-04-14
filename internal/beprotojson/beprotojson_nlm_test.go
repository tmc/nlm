package beprotojson

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	// These tests verify beprotojson works with actual NotebookLM protocol buffer types
	// For generic tests without nlm dependencies, see beprotojson_basic_test.go
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
			name: "project with chatbot config",
			json: `["project3", [], "id3", "📚", null, null, null, [[2, "Be precise"], [4]]]`,
			want: &pb.Project{
				Title:     "project3",
				ProjectId: "id3",
				Emoji:     "📚",
				ChatbotConfig: &pb.ChatbotConfig{
					Goal: &pb.ChatGoalConfig{
						Goal:         2,
						CustomPrompt: "Be precise",
					},
					ResponseLength: &pb.ResponseLengthConfig{
						Value: 4,
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

func TestUnmarshalProjectAnalytics(t *testing.T) {
	got := &pb.ProjectAnalytics{}
	if err := Unmarshal([]byte(`[[335], [12], [1], [1731910459, 665561000]]`), got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	want := &pb.ProjectAnalytics{
		SourceCount:        &wrapperspb.Int32Value{Value: 335},
		NoteCount:          &wrapperspb.Int32Value{Value: 12},
		AudioOverviewCount: &wrapperspb.Int32Value{Value: 1},
		LastAccessed: &timestamppb.Timestamp{
			Seconds: 1731910459,
			Nanos:   665561000,
		},
	}

	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Fatalf("Unmarshal() diff (-want +got):\n%s", diff)
	}
}

func TestUnmarshalGetNotesResponse(t *testing.T) {
	got := &pb.GetNotesResponse{}
	json := `[[["note-1", ["note-1", "hello", null, null, "Test Note", "<p>hello</p>", [2]]], ["note-2", ["note-2", "world", null, null, "Second Note", "<p>world</p>", [1]]]]]`
	if err := Unmarshal([]byte(json), got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	want := &pb.GetNotesResponse{
		Notes: []*pb.Note{
			{
				NoteId:      "note-1",
				ContentText: "hello",
				Title:       "Test Note",
				RichText:    "<p>hello</p>",
				NoteType:    []int32{2},
			},
			{
				NoteId:      "note-2",
				ContentText: "world",
				Title:       "Second Note",
				RichText:    "<p>world</p>",
				NoteType:    []int32{1},
			},
		},
	}

	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Fatalf("Unmarshal() diff (-want +got):\n%s", diff)
	}
}

func TestUnmarshalFeaturedProjectsResponse(t *testing.T) {
	got := &pb.ListFeaturedProjectsResponse{}
	json := `[[["Archive 1945", [], "34510332-d39c-499e-882d-e48393d612cd", "🏛️", null, null, null, [[2, "Use sources only"], [4]], null, null, null, null, null, null, [false, [["https://example.com/cover.png"]], "The Economist", "Witness history as it unfolded in 1945."]]]]`
	if err := Unmarshal([]byte(json), got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	want := &pb.ListFeaturedProjectsResponse{
		Projects: []*pb.FeaturedProject{{
			Title:     "Archive 1945",
			ProjectId: "34510332-d39c-499e-882d-e48393d612cd",
			Emoji:     "🏛️",
			ChatbotConfig: &pb.ChatbotConfig{
				Goal: &pb.ChatGoalConfig{
					Goal:         2,
					CustomPrompt: "Use sources only",
				},
				ResponseLength: &pb.ResponseLengthConfig{
					Value: 4,
				},
			},
			Presentation: &pb.FeaturedProjectPresentation{
				Curated: false,
				Images: []*pb.FeaturedProjectImage{{
					Url: "https://example.com/cover.png",
				}},
				Provider:    "The Economist",
				Description: "Witness history as it unfolded in 1945.",
			},
		}},
	}

	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Fatalf("Unmarshal() diff (-want +got):\n%s", diff)
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

func TestUnmarshalListResponse(t *testing.T) {
	tests := []struct {
		name  string
		json  string
		count int
	}{
		{
			name:  "multiple projects",
			json:  `[["Project A", [], "id-a", "📚"], ["Project B", [], "id-b", "🤖"]]`,
			count: 2,
		},
		{
			name:  "five projects with metadata",
			json:  `[["NB1", [], "id1", "📚", null, [1, false, true]], ["NB2", [], "id2", "🤖"], ["NB3", [], "id3", "📔"], ["NB4", [], "id4", "💻"], ["NB5", [], "id5", "🧠"]]`,
			count: 5,
		},
		{
			name:  "wrapped array (positional format)",
			json:  `[[["Project A", [], "id-a", "📚"], ["Project B", [], "id-b", "🤖"]]]`,
			count: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := &pb.ListRecentlyViewedProjectsResponse{}
			err := Unmarshal([]byte(tt.json), got)
			if err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if len(got.Projects) != tt.count {
				t.Fatalf("expected %d projects, got %d", tt.count, len(got.Projects))
			}
			if got.Projects[0].Title != "NB1" && got.Projects[0].Title != "Project A" {
				t.Errorf("projects[0].Title = %q, unexpected", got.Projects[0].Title)
			}
		})
	}
}

// TestRoundTrip tests marshaling and unmarshaling
func TestRoundTrip(t *testing.T) {
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
		{
			name: "project with sources",
			msg: &pb.Project{
				Title:     "My Project",
				ProjectId: "id1",
				Emoji:     "📚",
				Sources: []*pb.Source{
					{
						SourceId: &pb.SourceId{SourceId: "src1"},
						Title:    "Source One",
					},
				},
			},
		},
		{
			name: "project with chatbot config",
			msg: &pb.Project{
				Title:     "project3",
				ProjectId: "id3",
				Emoji:     "📚",
				ChatbotConfig: &pb.ChatbotConfig{
					Goal: &pb.ChatGoalConfig{
						Goal:         2,
						CustomPrompt: "Be precise",
					},
					ResponseLength: &pb.ResponseLengthConfig{
						Value: 4,
					},
				},
			},
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
				t.Fatalf("Unmarshal() error = %v", err)
			}

			if diff := cmp.Diff(tt.msg, got, protocmp.Transform()); diff != "" {
				t.Errorf("Round trip diff (-want +got):\n%s", diff)
			}
		})
	}
}

// TestMarshalProject tests Marshal output for nlm Project messages
func TestMarshalProject(t *testing.T) {
	tests := []struct {
		name string
		msg  proto.Message
		want string
	}{
		{
			name: "basic project",
			msg: &pb.Project{
				Title:     "My Project",
				ProjectId: "id1",
				Emoji:     "📚",
			},
			want: `["My Project",null,"id1","📚",null,null,null,null]`,
		},
		{
			name: "nil message",
			msg:  (*pb.Project)(nil),
			want: `null`,
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

