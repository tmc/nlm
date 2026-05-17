package api

import (
	"reflect"
	"testing"
)

func TestNormalizeYouTubeSourceURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "watch URL",
			input: "https://www.youtube.com/watch?v=dyTIt1HQ_aw&t=65s",
			want:  "https://www.youtube.com/watch?v=dyTIt1HQ_aw&t=65s",
		},
		{
			name:  "short URL",
			input: "https://youtu.be/dyTIt1HQ_aw",
			want:  "https://youtu.be/dyTIt1HQ_aw",
		},
		{
			name:  "legacy video ID",
			input: "dyTIt1HQ_aw",
			want:  "https://www.youtube.com/watch?v=dyTIt1HQ_aw",
		},
		{
			name:    "empty video ID",
			input:   "",
			wantErr: true,
		},
		{
			name:    "unsupported URL",
			input:   "https://example.com/watch?v=dyTIt1HQ_aw",
			wantErr: true,
		},
		{
			name:    "host containing youtube.com",
			input:   "https://notyoutube.com/watch?v=dyTIt1HQ_aw",
			wantErr: true,
		},
		{
			name:    "youtube.com suffix impostor",
			input:   "https://youtube.com.evil/watch?v=dyTIt1HQ_aw",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := normalizeYouTubeSourceURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizeYouTubeSourceURL(%q) returned nil error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeYouTubeSourceURL(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("normalizeYouTubeSourceURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsYouTubeURLRequiresYouTubeHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "youtube watch URL",
			input: "https://www.youtube.com/watch?v=dyTIt1HQ_aw",
			want:  true,
		},
		{
			name:  "youtube subdomain",
			input: "https://m.youtube.com/watch?v=dyTIt1HQ_aw",
			want:  true,
		},
		{
			name:  "short URL",
			input: "https://youtu.be/dyTIt1HQ_aw",
			want:  true,
		},
		{
			name:  "host containing youtube.com",
			input: "https://notyoutube.com/watch?v=dyTIt1HQ_aw",
		},
		{
			name:  "youtube.com suffix impostor",
			input: "https://youtube.com.evil/watch?v=dyTIt1HQ_aw",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isYouTubeURL(tt.input); got != tt.want {
				t.Fatalf("isYouTubeURL(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildYouTubeSourcePayloadUsesCurrentURLShape(t *testing.T) {
	t.Parallel()

	got := buildYouTubeSourcePayload("notebook-123", "https://www.youtube.com/watch?v=dyTIt1HQ_aw")
	want := []interface{}{
		[]interface{}{
			[]interface{}{
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				[]string{"https://www.youtube.com/watch?v=dyTIt1HQ_aw"},
				nil,
				nil,
				1,
			},
		},
		"notebook-123",
		[]int{2},
		[]interface{}{1, nil, nil, nil, nil, nil, nil, nil, nil, nil, []int{1}},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildYouTubeSourcePayload() = %#v, want %#v", got, want)
	}
}
