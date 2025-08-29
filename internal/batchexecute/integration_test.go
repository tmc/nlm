package batchexecute

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestErrorHandlingIntegration tests the complete error handling pipeline
func TestErrorHandlingIntegration(t *testing.T) {
	tests := []struct {
		name           string
		responseBody   string
		expectError    bool
		expectedErrMsg string
		expectedCode   int
		isRetryable    bool
	}{
		{
			name:           "Authentication error response",
			responseBody:   ")]}'\n277566",
			expectError:    true,
			expectedErrMsg: "Authentication required",
			expectedCode:   277566,
			isRetryable:    false,
		},
		{
			name:           "Rate limit error response",
			responseBody:   ")]}'\n324934",
			expectError:    true,
			expectedErrMsg: "Rate limit exceeded",
			expectedCode:   324934,
			isRetryable:    true,
		},
		{
			name:           "Resource not found error",
			responseBody:   ")]}'\n143",
			expectError:    true,
			expectedErrMsg: "Resource not found",
			expectedCode:   143,
			isRetryable:    false,
		},
		{
			name:           "JSON array with error code",
			responseBody:   ")]}'\n[[\"wrb.fr\",\"test\",\"277567\",null,null,null,\"generic\"]]",
			expectError:    true,
			expectedErrMsg: "Authentication token expired",
			expectedCode:   277567,
			isRetryable:    false,
		},
		{
			name:           "Success response with code 1",
			responseBody:   ")]}'\n1",
			expectError:    false,
			expectedErrMsg: "",
		},
		{
			name:           "Success response with code 0",
			responseBody:   ")]}'\n0",
			expectError:    false,
			expectedErrMsg: "",
		},
		{
			name:         "Normal notebook list response",
			responseBody: ")]}'\n[[\"wrb.fr\",\"test\",\"[{\\\"notebooks\\\":[{\\\"id\\\":\\\"123\\\",\\\"title\\\":\\\"Test Notebook\\\"}]}]\",null,null,null,\"generic\"]]",
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, tt.responseBody)
			}))
			defer server.Close()

			// Create client with test server
			config := Config{
				Host:      server.URL[7:], // Remove "http://" prefix
				App:       "test",
				AuthToken: "test-token",
				Cookies:   "test=cookie",
				UseHTTP:   true,
			}
			client := NewClient(config)

			// Execute a test RPC
			rpc := RPC{
				ID:   "test",
				Args: []interface{}{},
			}

			response, err := client.Do(rpc)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
					return
				}

				// Check if it's an APIError
				if apiErr, ok := err.(*APIError); ok {
					if apiErr.Message != tt.expectedErrMsg {
						t.Errorf("APIError.Message = %q, want %q", apiErr.Message, tt.expectedErrMsg)
					}
					
					if tt.expectedCode != 0 && (apiErr.ErrorCode == nil || apiErr.ErrorCode.Code != tt.expectedCode) {
						t.Errorf("APIError.ErrorCode.Code = %v, want %d", apiErr.ErrorCode, tt.expectedCode)
					}
					
					if apiErr.IsRetryable() != tt.isRetryable {
						t.Errorf("APIError.IsRetryable() = %v, want %v", apiErr.IsRetryable(), tt.isRetryable)
					}
				} else {
					t.Errorf("Expected APIError but got %T: %v", err, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
					return
				}
				
				if response == nil {
					t.Errorf("Expected response but got nil")
				}
			}
		})
	}
}

// TestHTTPStatusErrorHandling tests HTTP status code error handling
func TestHTTPStatusErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		expectError bool
		isRetryable bool
	}{
		{
			name:        "HTTP 429 Too Many Requests",
			statusCode:  429,
			expectError: true,
			isRetryable: true,
		},
		{
			name:        "HTTP 500 Internal Server Error",
			statusCode:  500,
			expectError: true,
			isRetryable: true,
		},
		{
			name:        "HTTP 401 Unauthorized",
			statusCode:  401,
			expectError: true,
			isRetryable: false,
		},
		{
			name:        "HTTP 403 Forbidden",
			statusCode:  403,
			expectError: true,
			isRetryable: false,
		},
		{
			name:        "HTTP 404 Not Found",
			statusCode:  404,
			expectError: true,
			isRetryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server that returns the specified HTTP status
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				fmt.Fprint(w, "Error response")
			}))
			defer server.Close()

			// Create client with test server
			config := Config{
				Host:      server.URL[7:], // Remove "http://" prefix
				App:       "test",
				AuthToken: "test-token",
				Cookies:   "test=cookie",
				UseHTTP:   true,
			}
			client := NewClient(config)

			// Execute a test RPC
			rpc := RPC{
				ID:   "test",
				Args: []interface{}{},
			}

			_, err := client.Do(rpc)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
					return
				}

				// Check if it's a BatchExecuteError (HTTP errors are handled differently)
				if batchErr, ok := err.(*BatchExecuteError); ok {
					if batchErr.StatusCode != tt.statusCode {
						t.Errorf("BatchExecuteError.StatusCode = %d, want %d", batchErr.StatusCode, tt.statusCode)
					}
				} else {
					t.Errorf("Expected BatchExecuteError but got %T: %v", err, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

// TestCustomErrorCodeExtension tests the ability to add custom error codes
func TestCustomErrorCodeExtension(t *testing.T) {
	// Save original state
	originalCodes := ListErrorCodes()
	
	// Add a custom error code
	customCode := 999999
	customError := ErrorCode{
		Code:        customCode,
		Type:        ErrorTypeServerError,
		Message:     "Custom service unavailable",
		Description: "A custom error for testing extensibility",
		Retryable:   true,
	}
	
	AddErrorCode(customCode, customError)
	
	// Test that the custom error code works in the pipeline
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, ")]}'\n%d", customCode)
	}))
	defer server.Close()

	// Create client with test server
	config := Config{
		Host:      server.URL[7:], // Remove "http://" prefix
		App:       "test",
		AuthToken: "test-token",
		Cookies:   "test=cookie",
		UseHTTP:   true,
	}
	client := NewClient(config)

	// Execute a test RPC
	rpc := RPC{
		ID:   "test",
		Args: []interface{}{},
	}

	_, err := client.Do(rpc)
	
	if err == nil {
		t.Errorf("Expected error but got none")
		return
	}

	// Check if it's an APIError with our custom error code
	if apiErr, ok := err.(*APIError); ok {
		if apiErr.ErrorCode == nil || apiErr.ErrorCode.Code != customCode {
			t.Errorf("APIError.ErrorCode.Code = %v, want %d", apiErr.ErrorCode, customCode)
		}
		
		if apiErr.Message != customError.Message {
			t.Errorf("APIError.Message = %q, want %q", apiErr.Message, customError.Message)
		}
		
		if !apiErr.IsRetryable() {
			t.Errorf("APIError.IsRetryable() = false, want true")
		}
	} else {
		t.Errorf("Expected APIError but got %T: %v", err, err)
	}
	
	// Clean up - restore original state
	errorCodeDictionary = make(map[int]ErrorCode)
	for code, errorCode := range originalCodes {
		errorCodeDictionary[code] = errorCode
	}
}