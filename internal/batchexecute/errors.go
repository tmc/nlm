package batchexecute

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ErrorType represents different categories of API errors
type ErrorType int

const (
	ErrorTypeUnknown ErrorType = iota
	ErrorTypeAuthentication
	ErrorTypeAuthorization
	ErrorTypeRateLimit
	ErrorTypeNotFound
	ErrorTypeInvalidInput
	ErrorTypeServerError
	ErrorTypeNetworkError
	ErrorTypePermissionDenied
	ErrorTypeResourceExhausted
	ErrorTypeUnavailable
)

// String returns the string representation of the ErrorType
func (e ErrorType) String() string {
	switch e {
	case ErrorTypeAuthentication:
		return "Authentication"
	case ErrorTypeAuthorization:
		return "Authorization"
	case ErrorTypeRateLimit:
		return "RateLimit"
	case ErrorTypeNotFound:
		return "NotFound"
	case ErrorTypeInvalidInput:
		return "InvalidInput"
	case ErrorTypeServerError:
		return "ServerError"
	case ErrorTypeNetworkError:
		return "NetworkError"
	case ErrorTypePermissionDenied:
		return "PermissionDenied"
	case ErrorTypeResourceExhausted:
		return "ResourceExhausted"
	case ErrorTypeUnavailable:
		return "Unavailable"
	default:
		return "Unknown"
	}
}

// ErrorCode represents a specific error code with its type and description
type ErrorCode struct {
	Code        int       `json:"code"`
	Type        ErrorType `json:"type"`
	Message     string    `json:"message"`
	Description string    `json:"description"`
	Retryable   bool      `json:"retryable"`
}

// APIError represents a parsed API error response
type APIError struct {
	ErrorCode   *ErrorCode `json:"error_code,omitempty"`
	HTTPStatus  int        `json:"http_status,omitempty"`
	RawResponse string     `json:"raw_response,omitempty"`
	Message     string     `json:"message"`
}

func (e *APIError) Error() string {
	if e.ErrorCode != nil {
		return fmt.Sprintf("API error %d (%s): %s", e.ErrorCode.Code, e.ErrorCode.Type, e.ErrorCode.Message)
	}
	if e.HTTPStatus != 0 {
		return fmt.Sprintf("HTTP error %d: %s", e.HTTPStatus, e.Message)
	}
	return fmt.Sprintf("API error: %s", e.Message)
}

// IsRetryable returns true if the error can be retried
func (e *APIError) IsRetryable() bool {
	if e.ErrorCode != nil {
		return e.ErrorCode.Retryable
	}
	// HTTP errors that are retryable
	switch e.HTTPStatus {
	case 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}

// errorCodeDictionary maps numeric error codes to their definitions
var errorCodeDictionary = map[int]ErrorCode{
	// Authentication errors
	277566: {
		Code:        277566,
		Type:        ErrorTypeAuthentication,
		Message:     "Authentication required",
		Description: "The request requires user authentication. Please run 'nlm auth' to authenticate.",
		Retryable:   false,
	},
	277567: {
		Code:        277567,
		Type:        ErrorTypeAuthentication,
		Message:     "Authentication token expired",
		Description: "The authentication token has expired. Please run 'nlm auth' to re-authenticate.",
		Retryable:   false,
	},
	80620: {
		Code:        80620,
		Type:        ErrorTypeAuthorization,
		Message:     "Access denied",
		Description: "Access to the requested resource is denied. Check your permissions.",
		Retryable:   false,
	},

	// Rate limiting
	324934: {
		Code:        324934,
		Type:        ErrorTypeRateLimit,
		Message:     "Rate limit exceeded",
		Description: "Too many requests have been sent. Please wait before making more requests.",
		Retryable:   true,
	},

	// Resource not found
	143: {
		Code:        143,
		Type:        ErrorTypeNotFound,
		Message:     "Resource not found",
		Description: "The requested resource could not be found. It may have been deleted or you may not have access to it.",
		Retryable:   false,
	},

	// Permission denied
	4: {
		Code:        4,
		Type:        ErrorTypePermissionDenied,
		Message:     "Permission denied",
		Description: "You do not have permission to access this resource.",
		Retryable:   false,
	},

	// Additional common error codes
	1: {
		Code:        1,
		Type:        ErrorTypeInvalidInput,
		Message:     "Invalid request",
		Description: "The request contains invalid parameters or data.",
		Retryable:   false,
	},
	2: {
		Code:        2,
		Type:        ErrorTypeServerError,
		Message:     "Internal server error",
		Description: "An internal server error occurred. Please try again later.",
		Retryable:   true,
	},
	3: {
		Code:        3,
		Type:        ErrorTypeUnavailable,
		Message:     "Service unavailable",
		Description: "The service is temporarily unavailable. Please try again later.",
		Retryable:   true,
	},
	5: {
		Code:        5,
		Type:        ErrorTypeNotFound,
		Message:     "Not found",
		Description: "The requested item was not found.",
		Retryable:   false,
	},
	6: {
		Code:        6,
		Type:        ErrorTypeInvalidInput,
		Message:     "Invalid argument",
		Description: "One or more arguments are invalid.",
		Retryable:   false,
	},
	7: {
		Code:        7,
		Type:        ErrorTypePermissionDenied,
		Message:     "Permission denied",
		Description: "The caller does not have permission to execute the specified operation.",
		Retryable:   false,
	},
	8: {
		Code:        8,
		Type:        ErrorTypeResourceExhausted,
		Message:     "Resource exhausted",
		Description: "Some resource has been exhausted (quota, disk space, etc.).",
		Retryable:   true,
	},
	9: {
		Code:        9,
		Type:        ErrorTypeInvalidInput,
		Message:     "Failed precondition",
		Description: "Operation was rejected because the system is not in a state required for the operation's execution.",
		Retryable:   false,
	},
	10: {
		Code:        10,
		Type:        ErrorTypeServerError,
		Message:     "Aborted",
		Description: "The operation was aborted due to a concurrency issue.",
		Retryable:   true,
	},
	11: {
		Code:        11,
		Type:        ErrorTypeInvalidInput,
		Message:     "Out of range",
		Description: "Operation was attempted past the valid range.",
		Retryable:   false,
	},
	12: {
		Code:        12,
		Type:        ErrorTypeServerError,
		Message:     "Unimplemented",
		Description: "Operation is not implemented or not supported/enabled.",
		Retryable:   false,
	},
	13: {
		Code:        13,
		Type:        ErrorTypeServerError,
		Message:     "Internal error",
		Description: "Internal errors that shouldn't be exposed to clients.",
		Retryable:   true,
	},
	14: {
		Code:        14,
		Type:        ErrorTypeUnavailable,
		Message:     "Unavailable",
		Description: "The service is currently unavailable.",
		Retryable:   true,
	},
	15: {
		Code:        15,
		Type:        ErrorTypeServerError,
		Message:     "Data loss",
		Description: "Unrecoverable data loss or corruption.",
		Retryable:   false,
	},
	16: {
		Code:        16,
		Type:        ErrorTypeAuthentication,
		Message:     "Unauthenticated",
		Description: "The request does not have valid authentication credentials.",
		Retryable:   false,
	},

	// HTTP status code mappings (for consistency)
	400: {
		Code:        400,
		Type:        ErrorTypeInvalidInput,
		Message:     "Bad Request",
		Description: "The request is malformed or contains invalid parameters.",
		Retryable:   false,
	},
	401: {
		Code:        401,
		Type:        ErrorTypeAuthentication,
		Message:     "Unauthorized",
		Description: "Authentication is required to access this resource.",
		Retryable:   false,
	},
	403: {
		Code:        403,
		Type:        ErrorTypePermissionDenied,
		Message:     "Forbidden",
		Description: "Access to this resource is forbidden.",
		Retryable:   false,
	},
	404: {
		Code:        404,
		Type:        ErrorTypeNotFound,
		Message:     "Not Found",
		Description: "The requested resource was not found.",
		Retryable:   false,
	},
	429: {
		Code:        429,
		Type:        ErrorTypeRateLimit,
		Message:     "Too Many Requests",
		Description: "Rate limit exceeded. Please wait before making more requests.",
		Retryable:   true,
	},
	500: {
		Code:        500,
		Type:        ErrorTypeServerError,
		Message:     "Internal Server Error",
		Description: "An internal server error occurred.",
		Retryable:   true,
	},
	502: {
		Code:        502,
		Type:        ErrorTypeServerError,
		Message:     "Bad Gateway",
		Description: "The server received an invalid response from an upstream server.",
		Retryable:   true,
	},
	503: {
		Code:        503,
		Type:        ErrorTypeUnavailable,
		Message:     "Service Unavailable",
		Description: "The service is temporarily unavailable.",
		Retryable:   true,
	},
	504: {
		Code:        504,
		Type:        ErrorTypeServerError,
		Message:     "Gateway Timeout",
		Description: "The server did not receive a timely response from an upstream server.",
		Retryable:   true,
	},
}

// GetErrorCode returns the ErrorCode for a given numeric code
func GetErrorCode(code int) (*ErrorCode, bool) {
	if errorCode, exists := errorCodeDictionary[code]; exists {
		return &errorCode, true
	}
	return nil, false
}

// IsErrorResponse checks if a response contains an error by examining the response data
func IsErrorResponse(response *Response) (*APIError, bool) {
	if response == nil {
		return nil, false
	}

	// Check if the response has an explicit error field
	if response.Error != "" {
		return &APIError{
			Message: response.Error,
		}, true
	}

	// Check if the response data contains error indicators
	if response.Data == nil {
		return nil, false
	}

	// Try to parse the response data as a numeric error code
	var rawData interface{}
	if err := json.Unmarshal(response.Data, &rawData); err != nil {
		return nil, false
	}

	// Handle different response data formats
	switch data := rawData.(type) {
	case float64:
		// Single numeric error code
		code := int(data)
		// Skip success codes (0 and 1 are typically success indicators)
		if code == 0 || code == 1 {
			return nil, false
		}
		if errorCode, exists := GetErrorCode(code); exists {
			return &APIError{
				ErrorCode: errorCode,
				Message:   errorCode.Message,
			}, true
		}
		// Unknown numeric error code (but not success codes)
		return &APIError{
			Message: fmt.Sprintf("Unknown error code: %d", code),
		}, true
	case []interface{}:
		// Array response - check first element for error codes
		if len(data) > 0 {
			if firstEl, ok := data[0].(float64); ok {
				code := int(firstEl)
				// Skip success codes
				if code == 0 || code == 1 {
					return nil, false
				}
				if errorCode, exists := GetErrorCode(code); exists {
					return &APIError{
						ErrorCode: errorCode,
						Message:   errorCode.Message,
					}, true
				}
			}
		}
	case map[string]interface{}:
		// Object response - check for error fields
		if errorMsg, ok := data["error"].(string); ok && errorMsg != "" {
			return &APIError{
				Message: errorMsg,
			}, true
		}
		if errorCode, ok := data["error_code"].(float64); ok {
			code := int(errorCode)
			if ec, exists := GetErrorCode(code); exists {
				return &APIError{
					ErrorCode: ec,
					Message:   ec.Message,
				}, true
			}
		}
	case string:
		// String response - check if it's a numeric error code
		if code, err := strconv.Atoi(strings.TrimSpace(data)); err == nil {
			// Skip success codes
			if code == 0 || code == 1 {
				return nil, false
			}
			if errorCode, exists := GetErrorCode(code); exists {
				return &APIError{
					ErrorCode: errorCode,
					Message:   errorCode.Message,
				}, true
			}
		}
	}

	return nil, false
}

// ParseAPIError attempts to extract error information from a raw response
func ParseAPIError(rawResponse string, httpStatus int) *APIError {
	// Try to parse as JSON first
	var rawData interface{}
	if err := json.Unmarshal([]byte(rawResponse), &rawData); err == nil {
		// Check for numeric error codes
		switch data := rawData.(type) {
		case float64:
			code := int(data)
			// Skip success codes
			if code != 0 && code != 1 {
				if errorCode, exists := GetErrorCode(code); exists {
					return &APIError{
						ErrorCode:   errorCode,
						HTTPStatus:  httpStatus,
						RawResponse: rawResponse,
						Message:     errorCode.Message,
					}
				}
			}
		case []interface{}:
			if len(data) > 0 {
				if firstEl, ok := data[0].(float64); ok {
					code := int(firstEl)
					// Skip success codes
					if code != 0 && code != 1 {
						if errorCode, exists := GetErrorCode(code); exists {
							return &APIError{
								ErrorCode:   errorCode,
								HTTPStatus:  httpStatus,
								RawResponse: rawResponse,
								Message:     errorCode.Message,
							}
						}
					}
				}
			}
		}
	}

	// Try to parse as a raw numeric error code
	if strings.TrimSpace(rawResponse) != "" {
		if code, err := strconv.Atoi(strings.TrimSpace(rawResponse)); err == nil {
			// Skip success codes
			if code != 0 && code != 1 {
				if errorCode, exists := GetErrorCode(code); exists {
					return &APIError{
						ErrorCode:   errorCode,
						HTTPStatus:  httpStatus,
						RawResponse: rawResponse,
						Message:     errorCode.Message,
					}
				}
			}
		}
	}

	// If we have an HTTP error status, use that
	if httpStatus >= 400 {
		if errorCode, exists := GetErrorCode(httpStatus); exists {
			return &APIError{
				ErrorCode:   errorCode,
				HTTPStatus:  httpStatus,
				RawResponse: rawResponse,
				Message:     errorCode.Message,
			}
		}
		return &APIError{
			HTTPStatus:  httpStatus,
			RawResponse: rawResponse,
			Message:     fmt.Sprintf("HTTP error %d", httpStatus),
		}
	}

	// Generic error
	return &APIError{
		HTTPStatus:  httpStatus,
		RawResponse: rawResponse,
		Message:     "Unknown API error",
	}
}

// AddErrorCode allows adding custom error codes to the dictionary at runtime
func AddErrorCode(code int, errorCode ErrorCode) {
	errorCodeDictionary[code] = errorCode
}

// ListErrorCodes returns all registered error codes
func ListErrorCodes() map[int]ErrorCode {
	result := make(map[int]ErrorCode)
	for k, v := range errorCodeDictionary {
		result[k] = v
	}
	return result
}
