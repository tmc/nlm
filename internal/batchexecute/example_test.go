package batchexecute

import (
	"fmt"
	"log"
)

// ExampleAPIError demonstrates how API errors are detected and handled
func ExampleAPIError() {
	// Example 1: Authentication error code
	authError := &APIError{
		ErrorCode: &ErrorCode{
			Code:        277566,
			Type:        ErrorTypeAuthentication,
			Message:     "Authentication required",
			Description: "The request requires user authentication. Please run 'nlm auth' to authenticate.",
			Retryable:   false,
		},
		Message: "Authentication required",
	}

	fmt.Printf("Error: %s\n", authError.Error())
	fmt.Printf("Retryable: %t\n", authError.IsRetryable())

	// Example 2: Rate limit error (retryable)
	rateLimitError := &APIError{
		ErrorCode: &ErrorCode{
			Code:        324934,
			Type:        ErrorTypeRateLimit,
			Message:     "Rate limit exceeded",
			Description: "Too many requests have been sent. Please wait before making more requests.",
			Retryable:   true,
		},
		Message: "Rate limit exceeded",
	}

	fmt.Printf("Error: %s\n", rateLimitError.Error())
	fmt.Printf("Retryable: %t\n", rateLimitError.IsRetryable())

	// Output:
	// Error: API error 277566 (Authentication): Authentication required
	// Retryable: false
	// Error: API error 324934 (RateLimit): Rate limit exceeded
	// Retryable: true
}

// ExampleGetErrorCode demonstrates how to look up error codes
func ExampleGetErrorCode() {
	// Look up a known error code
	if errorCode, exists := GetErrorCode(277566); exists {
		fmt.Printf("Code: %d\n", errorCode.Code)
		fmt.Printf("Type: %s\n", errorCode.Type)
		fmt.Printf("Message: %s\n", errorCode.Message)
		fmt.Printf("Retryable: %t\n", errorCode.Retryable)
	}

	// Look up an unknown error code
	if _, exists := GetErrorCode(999999); !exists {
		fmt.Println("Error code 999999 not found")
	}

	// Output:
	// Code: 277566
	// Type: Authentication
	// Message: Authentication required
	// Retryable: false
	// Error code 999999 not found
}

// ExampleAddErrorCode demonstrates how to extend the error dictionary
func ExampleAddErrorCode() {
	// Add a custom error code
	customError := ErrorCode{
		Code:        123456,
		Type:        ErrorTypeServerError,
		Message:     "Custom server error",
		Description: "A custom error for our application",
		Retryable:   true,
	}

	AddErrorCode(123456, customError)

	// Now we can look it up
	if errorCode, exists := GetErrorCode(123456); exists {
		fmt.Printf("Custom error: %s\n", errorCode.Message)
		fmt.Printf("Type: %s\n", errorCode.Type)
	}

	// Output:
	// Custom error: Custom server error
	// Type: ServerError
}

// ExampleIsErrorResponse demonstrates automatic error detection
func ExampleIsErrorResponse() {
	// This would typically be called automatically during response processing

	// Example with a numeric error code response
	response := &Response{
		ID:   "test",
		Data: []byte("277566"), // Authentication error code
	}

	if apiError, isError := IsErrorResponse(response); isError {
		log.Printf("Detected error: %s", apiError.Error())
		if apiError.ErrorCode != nil {
			log.Printf("Error code: %d", apiError.ErrorCode.Code)
			log.Printf("Error type: %s", apiError.ErrorCode.Type)
		}
	}

	// Example with a success response
	successResponse := &Response{
		ID:   "test",
		Data: []byte(`{"notebooks": [{"id": "123", "title": "Test"}]}`),
	}

	if _, isError := IsErrorResponse(successResponse); !isError {
		log.Println("Success response detected")
	}
}