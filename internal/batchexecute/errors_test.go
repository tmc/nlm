package batchexecute

import (
	"encoding/json"
	"testing"
)

func TestGetErrorCode(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		wantType ErrorType
		wantMsg  string
		exists   bool
	}{
		{
			name:     "Authentication required",
			code:     277566,
			wantType: ErrorTypeAuthentication,
			wantMsg:  "Authentication required",
			exists:   true,
		},
		{
			name:     "Authentication token expired",
			code:     277567,
			wantType: ErrorTypeAuthentication,
			wantMsg:  "Authentication token expired",
			exists:   true,
		},
		{
			name:     "Rate limit exceeded",
			code:     324934,
			wantType: ErrorTypeRateLimit,
			wantMsg:  "Rate limit exceeded",
			exists:   true,
		},
		{
			name:     "Resource not found",
			code:     143,
			wantType: ErrorTypeNotFound,
			wantMsg:  "Resource not found",
			exists:   true,
		},
		{
			name:     "Permission denied",
			code:     4,
			wantType: ErrorTypePermissionDenied,
			wantMsg:  "Permission denied",
			exists:   true,
		},
		{
			name:     "HTTP 429 Too Many Requests",
			code:     429,
			wantType: ErrorTypeRateLimit,
			wantMsg:  "Too Many Requests",
			exists:   true,
		},
		{
			name:     "HTTP 500 Internal Server Error",
			code:     500,
			wantType: ErrorTypeServerError,
			wantMsg:  "Internal Server Error",
			exists:   true,
		},
		{
			name:     "Unknown error code",
			code:     999999,
			wantType: ErrorTypeUnknown,
			wantMsg:  "",
			exists:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errorCode, exists := GetErrorCode(tt.code)

			if exists != tt.exists {
				t.Errorf("GetErrorCode(%d) exists = %v, want %v", tt.code, exists, tt.exists)
			}

			if tt.exists {
				if errorCode == nil {
					t.Errorf("GetErrorCode(%d) returned nil errorCode but exists = true", tt.code)
					return
				}

				if errorCode.Type != tt.wantType {
					t.Errorf("GetErrorCode(%d).Type = %v, want %v", tt.code, errorCode.Type, tt.wantType)
				}

				if errorCode.Message != tt.wantMsg {
					t.Errorf("GetErrorCode(%d).Message = %q, want %q", tt.code, errorCode.Message, tt.wantMsg)
				}

				if errorCode.Code != tt.code {
					t.Errorf("GetErrorCode(%d).Code = %d, want %d", tt.code, errorCode.Code, tt.code)
				}
			}
		})
	}
}

func TestIsErrorResponse(t *testing.T) {
	tests := []struct {
		name         string
		response     *Response
		wantError    bool
		wantErrorMsg string
		wantCode     int
	}{
		{
			name:         "Nil response",
			response:     nil,
			wantError:    false,
			wantErrorMsg: "",
		},
		{
			name: "Response with explicit error field",
			response: &Response{
				ID:    "test",
				Error: "Something went wrong",
			},
			wantError:    true,
			wantErrorMsg: "Something went wrong",
		},
		{
			name: "Numeric error code 277566",
			response: &Response{
				ID:   "test",
				Data: json.RawMessage("277566"),
			},
			wantError:    true,
			wantErrorMsg: "Authentication required",
			wantCode:     277566,
		},
		{
			name: "Numeric error code 324934 (rate limit)",
			response: &Response{
				ID:   "test",
				Data: json.RawMessage("324934"),
			},
			wantError:    true,
			wantErrorMsg: "Rate limit exceeded",
			wantCode:     324934,
		},
		{
			name: "Array with error code as first element",
			response: &Response{
				ID:   "test",
				Data: json.RawMessage("[143, \"additional\", \"data\"]"),
			},
			wantError:    true,
			wantErrorMsg: "Resource not found",
			wantCode:     143,
		},
		{
			name: "Object with error field",
			response: &Response{
				ID:   "test",
				Data: json.RawMessage(`{"error": "Custom error message"}`),
			},
			wantError:    true,
			wantErrorMsg: "Custom error message",
		},
		{
			name: "Object with error_code field",
			response: &Response{
				ID:   "test",
				Data: json.RawMessage(`{"error_code": 4, "message": "Access denied"}`),
			},
			wantError:    true,
			wantErrorMsg: "Permission denied",
			wantCode:     4,
		},
		{
			name: "String numeric error code",
			response: &Response{
				ID:   "test",
				Data: json.RawMessage(`"277567"`),
			},
			wantError:    true,
			wantErrorMsg: "Authentication token expired",
			wantCode:     277567,
		},
		{
			name: "Success response with code 0",
			response: &Response{
				ID:   "test",
				Data: json.RawMessage("0"),
			},
			wantError: false,
		},
		{
			name: "Success response with code 1",
			response: &Response{
				ID:   "test",
				Data: json.RawMessage("1"),
			},
			wantError: false,
		},
		{
			name: "Normal success response",
			response: &Response{
				ID:   "test",
				Data: json.RawMessage(`{"notebooks": [{"id": "123", "title": "Test"}]}`),
			},
			wantError: false,
		},
		{
			name: "Unknown numeric error code",
			response: &Response{
				ID:   "test",
				Data: json.RawMessage("999999"),
			},
			wantError:    true,
			wantErrorMsg: "Unknown error code: 999999",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiError, isError := IsErrorResponse(tt.response)

			if isError != tt.wantError {
				t.Errorf("IsErrorResponse() isError = %v, want %v", isError, tt.wantError)
			}

			if tt.wantError {
				if apiError == nil {
					t.Errorf("IsErrorResponse() returned nil apiError but isError = true")
					return
				}

				if apiError.Message != tt.wantErrorMsg {
					t.Errorf("IsErrorResponse() apiError.Message = %q, want %q", apiError.Message, tt.wantErrorMsg)
				}

				if tt.wantCode != 0 {
					if apiError.ErrorCode == nil {
						t.Errorf("IsErrorResponse() apiError.ErrorCode = nil, want code %d", tt.wantCode)
					} else if apiError.ErrorCode.Code != tt.wantCode {
						t.Errorf("IsErrorResponse() apiError.ErrorCode.Code = %d, want %d", apiError.ErrorCode.Code, tt.wantCode)
					}
				}
			}
		})
	}
}

func TestParseAPIError(t *testing.T) {
	tests := []struct {
		name          string
		rawResponse   string
		httpStatus    int
		wantErrorMsg  string
		wantCode      int
		wantRetryable bool
	}{
		{
			name:          "Numeric error code 277566",
			rawResponse:   "277566",
			httpStatus:    200,
			wantErrorMsg:  "Authentication required",
			wantCode:      277566,
			wantRetryable: false,
		},
		{
			name:          "JSON array with error code",
			rawResponse:   "[324934]",
			httpStatus:    200,
			wantErrorMsg:  "Rate limit exceeded",
			wantCode:      324934,
			wantRetryable: true,
		},
		{
			name:          "HTTP 429 error",
			rawResponse:   "Too Many Requests",
			httpStatus:    429,
			wantErrorMsg:  "Too Many Requests",
			wantCode:      429,
			wantRetryable: true,
		},
		{
			name:          "HTTP 500 error",
			rawResponse:   "Internal Server Error",
			httpStatus:    500,
			wantErrorMsg:  "Internal Server Error",
			wantCode:      500,
			wantRetryable: true,
		},
		{
			name:          "Unknown numeric error",
			rawResponse:   "123456",
			httpStatus:    200,
			wantErrorMsg:  "Unknown API error",
			wantCode:      0,
			wantRetryable: false,
		},
		{
			name:          "Generic error",
			rawResponse:   "Something went wrong",
			httpStatus:    200,
			wantErrorMsg:  "Unknown API error",
			wantCode:      0,
			wantRetryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiError := ParseAPIError(tt.rawResponse, tt.httpStatus)

			if apiError == nil {
				t.Errorf("ParseAPIError() returned nil")
				return
			}

			if apiError.Message != tt.wantErrorMsg {
				t.Errorf("ParseAPIError() Message = %q, want %q", apiError.Message, tt.wantErrorMsg)
			}

			if tt.wantCode != 0 {
				if apiError.ErrorCode == nil {
					t.Errorf("ParseAPIError() ErrorCode = nil, want code %d", tt.wantCode)
				} else if apiError.ErrorCode.Code != tt.wantCode {
					t.Errorf("ParseAPIError() ErrorCode.Code = %d, want %d", apiError.ErrorCode.Code, tt.wantCode)
				}
			}

			if apiError.IsRetryable() != tt.wantRetryable {
				t.Errorf("ParseAPIError() IsRetryable() = %v, want %v", apiError.IsRetryable(), tt.wantRetryable)
			}

			if apiError.HTTPStatus != tt.httpStatus {
				t.Errorf("ParseAPIError() HTTPStatus = %d, want %d", apiError.HTTPStatus, tt.httpStatus)
			}

			if apiError.RawResponse != tt.rawResponse {
				t.Errorf("ParseAPIError() RawResponse = %q, want %q", apiError.RawResponse, tt.rawResponse)
			}
		})
	}
}

func TestAPIError_Error(t *testing.T) {
	tests := []struct {
		name     string
		apiError *APIError
		want     string
	}{
		{
			name: "Error with ErrorCode",
			apiError: &APIError{
				ErrorCode: &ErrorCode{
					Code:    277566,
					Type:    ErrorTypeAuthentication,
					Message: "Authentication required",
				},
				Message: "Authentication required",
			},
			want: "API error 277566 (Authentication): Authentication required",
		},
		{
			name: "Error with HTTP status only",
			apiError: &APIError{
				HTTPStatus: 429,
				Message:    "Too Many Requests",
			},
			want: "HTTP error 429: Too Many Requests",
		},
		{
			name: "Generic error",
			apiError: &APIError{
				Message: "Something went wrong",
			},
			want: "API error: Something went wrong",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.apiError.Error()
			if got != tt.want {
				t.Errorf("APIError.Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestErrorType_String(t *testing.T) {
	tests := []struct {
		errorType ErrorType
		want      string
	}{
		{ErrorTypeAuthentication, "Authentication"},
		{ErrorTypeAuthorization, "Authorization"},
		{ErrorTypeRateLimit, "RateLimit"},
		{ErrorTypeNotFound, "NotFound"},
		{ErrorTypeInvalidInput, "InvalidInput"},
		{ErrorTypeServerError, "ServerError"},
		{ErrorTypeNetworkError, "NetworkError"},
		{ErrorTypePermissionDenied, "PermissionDenied"},
		{ErrorTypeResourceExhausted, "ResourceExhausted"},
		{ErrorTypeUnavailable, "Unavailable"},
		{ErrorTypeUnknown, "Unknown"},
		{ErrorType(999), "Unknown"}, // Test unknown error type
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.errorType.String()
			if got != tt.want {
				t.Errorf("ErrorType.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAddErrorCode(t *testing.T) {
	// Save original state
	originalCodes := ListErrorCodes()

	// Test adding a custom error code
	customCode := 999999
	customError := ErrorCode{
		Code:        customCode,
		Type:        ErrorTypeServerError,
		Message:     "Custom test error",
		Description: "This is a test error code",
		Retryable:   true,
	}

	AddErrorCode(customCode, customError)

	// Verify it was added
	retrievedError, exists := GetErrorCode(customCode)
	if !exists {
		t.Errorf("AddErrorCode() failed to add custom error code %d", customCode)
	}

	if retrievedError.Message != customError.Message {
		t.Errorf("AddErrorCode() Message = %q, want %q", retrievedError.Message, customError.Message)
	}

	if retrievedError.Type != customError.Type {
		t.Errorf("AddErrorCode() Type = %v, want %v", retrievedError.Type, customError.Type)
	}

	// Clean up - restore original state
	errorCodeDictionary = make(map[int]ErrorCode)
	for code, errorCode := range originalCodes {
		errorCodeDictionary[code] = errorCode
	}
}

func TestListErrorCodes(t *testing.T) {
	codes := ListErrorCodes()

	// Check that we have the expected error codes
	expectedCodes := []int{277566, 277567, 80620, 324934, 143, 4, 429, 500, 502, 503, 504}

	for _, expectedCode := range expectedCodes {
		if _, exists := codes[expectedCode]; !exists {
			t.Errorf("ListErrorCodes() missing expected error code %d", expectedCode)
		}
	}

	// Verify that modifying the returned map doesn't affect the original
	originalCount := len(codes)
	codes[999999] = ErrorCode{Code: 999999, Message: "Test"}

	newCodes := ListErrorCodes()
	if len(newCodes) != originalCount {
		t.Errorf("ListErrorCodes() returned map is not a copy, modifications affected original")
	}
}
