package batchexecute

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "connection refused",
			err:      errors.New("dial tcp 127.0.0.1:8080: connect: connection refused"),
			expected: true,
		},
		{
			name:     "i/o timeout",
			err:      errors.New("read tcp 192.168.1.1:443: i/o timeout"),
			expected: true,
		},
		{
			name:     "EOF error",
			err:      errors.New("unexpected EOF"),
			expected: true,
		},
		{
			name:     "non-retryable error",
			err:      errors.New("invalid argument"),
			expected: false,
		},
		{
			name:     "TLS handshake timeout",
			err:      errors.New("net/http: TLS handshake timeout"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetryableError(tt.err)
			if result != tt.expected {
				t.Errorf("isRetryableError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestIsRetryableStatus(t *testing.T) {
	tests := []struct {
		statusCode int
		expected   bool
	}{
		{http.StatusOK, false},
		{http.StatusBadRequest, false},
		{http.StatusUnauthorized, false},
		{http.StatusTooManyRequests, true},
		{http.StatusInternalServerError, true},
		{http.StatusBadGateway, true},
		{http.StatusServiceUnavailable, true},
		{http.StatusGatewayTimeout, true},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.statusCode), func(t *testing.T) {
			result := isRetryableStatus(tt.statusCode)
			if result != tt.expected {
				t.Errorf("isRetryableStatus(%d) = %v, want %v", tt.statusCode, result, tt.expected)
			}
		})
	}
}

func TestExecuteWithRetry(t *testing.T) {
	t.Run("successful on first attempt", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`)]}'
123
[[["wrb.fr","test","{\"result\":\"success\"}",null,null,null,"generic"]]]
`))
		}))
		defer server.Close()

		config := Config{
			Host:       server.URL[7:], // Remove http://
			App:        "test",
			MaxRetries: 3,
			RetryDelay: 10 * time.Millisecond,
			UseHTTP:    true,
		}
		client := NewClient(config)

		resp, err := client.Execute([]RPC{{ID: "test", Args: []interface{}{}}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp == nil {
			t.Fatal("expected response, got nil")
		}
	})

	t.Run("retry on temporary failure", func(t *testing.T) {
		var attempts int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := atomic.AddInt32(&attempts, 1)
			if count < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`)]}'
123
[[["wrb.fr","test","{\"result\":\"success\"}",null,null,null,"generic"]]]
`))
		}))
		defer server.Close()

		config := Config{
			Host:       server.URL[7:], // Remove http://
			App:        "test",
			MaxRetries: 3,
			RetryDelay: 10 * time.Millisecond,
			UseHTTP:    true,
		}
		client := NewClient(config)

		resp, err := client.Execute([]RPC{{ID: "test", Args: []interface{}{}}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp == nil {
			t.Fatal("expected response, got nil")
		}
		if atomic.LoadInt32(&attempts) != 3 {
			t.Errorf("expected 3 attempts, got %d", atomic.LoadInt32(&attempts))
		}
	})

	t.Run("fail after max retries", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		config := Config{
			Host:       server.URL[7:], // Remove http://
			App:        "test",
			MaxRetries: 2,
			RetryDelay: 10 * time.Millisecond,
			UseHTTP:    true,
		}
		client := NewClient(config)

		_, err := client.Execute([]RPC{{ID: "test", Args: []interface{}{}}})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if _, ok := err.(*BatchExecuteError); !ok {
			t.Errorf("expected BatchExecuteError, got %T", err)
		}
	})
}