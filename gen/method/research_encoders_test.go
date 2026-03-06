package method

import (
	"encoding/json"
	"testing"
)

func TestEncodeStartFastResearchArgs(t *testing.T) {
	tests := []struct {
		name       string
		projectID  string
		query      string
		sourceType int
		expected   string
	}{
		{
			name:       "Basic fast research query (web)",
			projectID:  "proj-123",
			query:      "AI in healthcare",
			sourceType: 1,
			expected:   `[["AI in healthcare",1],null,1,"proj-123"]`,
		},
		{
			name:       "Fast research with special characters (web)",
			projectID:  "proj-456",
			query:      "What is machine learning?",
			sourceType: 1,
			expected:   `[["What is machine learning?",1],null,1,"proj-456"]`,
		},
		{
			name:       "Fast research with Drive source type",
			projectID:  "proj-123",
			query:      "AI in healthcare",
			sourceType: 2,
			expected:   `[["AI in healthcare",2],null,2,"proj-123"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EncodeStartFastResearchArgs(tt.projectID, tt.query, tt.sourceType)
			resultJSON, err := json.Marshal(result)
			if err != nil {
				t.Fatalf("Failed to marshal result: %v", err)
			}
			if string(resultJSON) != tt.expected {
				t.Errorf("EncodeStartFastResearchArgs() = %s, want %s", string(resultJSON), tt.expected)
			}
		})
	}
}

func TestEncodeStartDeepResearchArgs(t *testing.T) {
	tests := []struct {
		name       string
		projectID  string
		query      string
		sourceType int
		expected   string
	}{
		{
			name:       "Basic deep research query (web)",
			projectID:  "proj-123",
			query:      "Climate change impacts",
			sourceType: 1,
			expected:   `[null,[1],["Climate change impacts",1],5,"proj-123"]`,
		},
		{
			name:       "Deep research with empty query (web)",
			projectID:  "proj-789",
			query:      "",
			sourceType: 1,
			expected:   `[null,[1],["",1],5,"proj-789"]`,
		},
		{
			name:       "Deep research with Drive source type",
			projectID:  "proj-123",
			query:      "Climate change impacts",
			sourceType: 2,
			expected:   `[null,[2],["Climate change impacts",2],5,"proj-123"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EncodeStartDeepResearchArgs(tt.projectID, tt.query, tt.sourceType)
			resultJSON, err := json.Marshal(result)
			if err != nil {
				t.Fatalf("Failed to marshal result: %v", err)
			}
			if string(resultJSON) != tt.expected {
				t.Errorf("EncodeStartDeepResearchArgs() = %s, want %s", string(resultJSON), tt.expected)
			}
		})
	}
}

func TestEncodePollResearchResultsArgs(t *testing.T) {
	tests := []struct {
		name      string
		projectID string
		expected  string
	}{
		{
			name:      "Poll research results",
			projectID: "proj-123",
			expected:  `[null,null,"proj-123"]`,
		},
		{
			name:      "Poll with different project ID",
			projectID: "another-proj-456",
			expected:  `[null,null,"another-proj-456"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EncodePollResearchResultsArgs(tt.projectID)
			resultJSON, err := json.Marshal(result)
			if err != nil {
				t.Fatalf("Failed to marshal result: %v", err)
			}
			if string(resultJSON) != tt.expected {
				t.Errorf("EncodePollResearchResultsArgs() = %s, want %s", string(resultJSON), tt.expected)
			}
		})
	}
}

func TestEncodeImportResearchSourcesArgs(t *testing.T) {
	tests := []struct {
		name       string
		projectID  string
		taskID     string
		sources    []string
		sourceType int
		expected   string
	}{
		{
			name:       "Import single source (web)",
			projectID:  "proj-123",
			taskID:     "task-abc",
			sources:    []string{"https://example.com/article1"},
			sourceType: 1,
			expected:   `[null,[1],"task-abc","proj-123",["https://example.com/article1"]]`,
		},
		{
			name:       "Import multiple sources (web)",
			projectID:  "proj-456",
			taskID:     "task-def",
			sources:    []string{"https://example.com/article1", "https://example.com/article2", "https://example.com/article3"},
			sourceType: 1,
			expected:   `[null,[1],"task-def","proj-456",["https://example.com/article1","https://example.com/article2","https://example.com/article3"]]`,
		},
		{
			name:       "Import with empty sources (web)",
			projectID:  "proj-789",
			taskID:     "task-ghi",
			sources:    []string{},
			sourceType: 1,
			expected:   `[null,[1],"task-ghi","proj-789",[]]`,
		},
		{
			name:       "Import single source (drive)",
			projectID:  "proj-123",
			taskID:     "task-abc",
			sources:    []string{"https://example.com/article1"},
			sourceType: 2,
			expected:   `[null,[2],"task-abc","proj-123",["https://example.com/article1"]]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EncodeImportResearchSourcesArgs(tt.projectID, tt.taskID, tt.sources, tt.sourceType)
			resultJSON, err := json.Marshal(result)
			if err != nil {
				t.Fatalf("Failed to marshal result: %v", err)
			}
			if string(resultJSON) != tt.expected {
				t.Errorf("EncodeImportResearchSourcesArgs() = %s, want %s", string(resultJSON), tt.expected)
			}
		})
	}
}
