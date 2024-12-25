package batchexecute

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

//go:embed testdata/*txt
var testdata embed.FS

func TestDecodeResponse(t *testing.T) {
	testCases := []struct {
		name      string
		input     string
		inputFile string
		chunked   bool
		expected  []Response
		validate  func(t *testing.T, resp []Response) // Optional validation function
		err       error
	}{
		{
			name:      "List Notebooks Response",
			inputFile: "list_notebooks.txt",
			chunked:   false,
			validate: func(t *testing.T, resp []Response) {
				if len(resp) != 1 {
					t.Errorf("Expected 1 response, got %d", len(resp))
					return
				}
			},
			err: nil,
		},
		{
			name: "Error Response",
			input: `123
[["wrb.fr","error","[{\"error\":\"Invalid request\",\"code\":400}]",null,null,null,"generic"]]`,
			chunked: true,
			expected: []Response{
				{
					ID:    "error",
					Index: 0,
					Data:  json.RawMessage(`[{"error":"Invalid request","code":400}]`),
				},
			},
			err: nil,
		},
		{
			name: "Multiple Chunk Types",
			input: `145
[["wrb.fr","VUsiyb","[null,null,[3,null,\"fec1780c-5a14-4f07-8ee6-f8c3ee2930fa\",\"nbname2\",null,true],null,[false]]",null,null,null,"generic"]]
25
[["e",4,null,null,237]]
58
[["di",125],["af.httprm",124,"6343297907846200142",27]]`,
			chunked: true,
			validate: func(t *testing.T, resp []Response) {
				if len(resp) != 1 {
					t.Errorf("Expected 1 response, got %d", len(resp))
					return
				}

				// Verify the main response
				if resp[0].ID != "VUsiyb" {
					t.Errorf("Expected ID VUsiyb, got %s", resp[0].ID)
				}
			},
			err: nil,
		},
		{
			name: "Deeply Nested JSON",
			input: `250
[["wrb.fr","nested","[{\"data\":{\"items\":[{\"id\":\"test\",\"metadata\":{\"created\":1234567890,\"modified\":1234567891},\"content\":{\"text\":\"Hello, World!\",\"format\":\"plain\"}}]}}]",null,null,null,"generic"]]`,
			chunked: true,
			validate: func(t *testing.T, resp []Response) {
				if len(resp) != 1 {
					t.Errorf("Expected 1 response, got %d", len(resp))
					return
				}

				// Verify the nested structure can be parsed
				var data struct {
					Data struct {
						Items []struct {
							ID       string `json:"id"`
							Metadata struct {
								Created  int64 `json:"created"`
								Modified int64 `json:"modified"`
							} `json:"metadata"`
							Content struct {
								Text   string `json:"text"`
								Format string `json:"format"`
							} `json:"content"`
						} `json:"items"`
					} `json:"data"`
				}

				if err := json.Unmarshal(resp[0].Data, &data); err != nil {
					t.Errorf("Failed to parse nested data: %v", err)
				}
			},
			err: nil,
		},
		{
			name:  "YouTube Source Addition Response",
			input: `)]}'\n105\n[["wrb.fr","izAoDd",null,null,null,[3],"generic"]]\n6\n[["e",4,null,null,237]]`,
			expected: []Response{
				{
					ID:    "izAoDd",
					Index: 0,
					Data:  json.RawMessage(`null`),
				},
			},
			err: nil,
		},
		{
			name: "Invalid Chunk Length",
			input: `abc
[["wrb.fr","test","data",null,null,null,"generic"]]`,
			chunked:  true,
			expected: nil,
			err:      fmt.Errorf("invalid chunk length: invalid syntax"),
		},
		{
			name: "Incomplete Chunk",
			input: `100
[["wrb.fr","test","`,
			chunked:  true,
			expected: nil,
			err:      fmt.Errorf("read chunk: unexpected EOF"),
		},
		{
			name:     "Empty Response",
			input:    "",
			chunked:  true,
			expected: nil,
			err:      fmt.Errorf("empty response after trimming prefix"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			if tc.inputFile != "" {
				content, err := testdata.ReadFile("testdata/" + tc.inputFile)
				if err != nil {
					t.Errorf("Failed to read test data: %v", err)
					return
				}
				tc.input = string(content)
			}

			var (
				actual []Response
				err    error
			)

			if tc.chunked {
				t.Skip("Chunked responses are in progress (please help!)")
				actual, err = decodeChunkedResponse(")]}'\n" + tc.input)
			} else {
				actual, err = decodeResponse(tc.input)
			}

			// Check error
			if !cmp.Equal(err, tc.err, cmpopts.EquateErrors()) {
				t.Errorf("Error mismatch (-want +got):\n%s", cmp.Diff(tc.err, err, cmpopts.EquateErrors()))
			}

			// If there's a validation function, use it
			if err == nil && tc.validate != nil {
				tc.validate(t, actual)
			}

			// If there are expected responses, compare them
			if err == nil && tc.expected != nil && !cmp.Equal(actual, tc.expected) {
				t.Errorf("Response mismatch (-want +got):\n%s", cmp.Diff(tc.expected, actual))
			}
		})
	}
}

func TestExecute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Log("Received request")

		// Verify request format
		if err := r.ParseForm(); err != nil {
			t.Errorf("Failed to parse form: %v", err)
			return
		}

		if r.Form.Get("f.req") == "" {
			t.Error("Missing f.req parameter")
			return
		}

		w.WriteHeader(http.StatusOK)
		// Return realistic response format
		fmt.Fprintf(w, `)]}'

[["wrb.fr","VUsiyb","[null,null,[3,null,\"fec1780c-5a14-4f07-8ee6-f8c3ee2930fa\",\"nbname2\",null,true],null,[false]]",null,null,null,"generic"]]`)
	}))
	defer server.Close()

	config := Config{
		Host:      strings.TrimPrefix(server.URL, "http://"),
		App:       "notebooklm",
		AuthToken: "test_token",
		Headers:   map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		UseHTTP:   true,
	}
	client := NewClient(config, WithHTTPClient(server.Client()))

	rpc := RPC{
		ID:    "VUsiyb",
		Args:  []interface{}{nil, 1},
		Index: "generic",
	}

	response, err := client.Execute([]RPC{rpc})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
		return
	}

	expectedData := json.RawMessage(`[null,null,[3,null,"fec1780c-5a14-4f07-8ee6-f8c3ee2930fa","nbname2",null,true],null,[false]]`)
	if string(response.Data) != string(expectedData) {
		t.Errorf("Unexpected response data:\ngot:  %s\nwant: %s", string(response.Data), string(expectedData))
	}
}
