package middleware

import (
	"encoding/json"
	"go-drive-duplicates/internal/interfaces/presenters"
	"log"
	"net/http"
	"runtime/debug"
	"time"
)

// ErrorHandlerMiddleware provides centralized error handling and recovery
func ErrorHandlerMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("❌ Panic recovered in %s %s: %v", r.Method, r.URL.Path, err)
				log.Printf("Stack trace: %s", debug.Stack())

				// Send internal server error response
				sendErrorResponse(w, "Internal server error", "INTERNAL_ERROR", http.StatusInternalServerError)
			}
		}()

		// Wrap response writer to capture error responses
		errorHandler := &ErrorResponseWriter{
			ResponseWriter: w,
			Request:        r,
		}

		next(errorHandler, r)
	}
}

// ValidationMiddleware validates common request parameters
func ValidationMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Validate Content-Type for POST/PUT requests
		if r.Method == http.MethodPost || r.Method == http.MethodPut {
			contentType := r.Header.Get("Content-Type")
			if contentType != "" && contentType != "application/json" && contentType != "application/x-www-form-urlencoded" {
				sendErrorResponse(w, "Unsupported Content-Type", "INVALID_CONTENT_TYPE", http.StatusUnsupportedMediaType)
				return
			}
		}

		// Validate request size (limit to 10MB)
		const maxRequestSize = 10 * 1024 * 1024
		if r.ContentLength > maxRequestSize {
			sendErrorResponse(w, "Request too large", "REQUEST_TOO_LARGE", http.StatusRequestEntityTooLarge)
			return
		}

		next(w, r)
	}
}

// CORSMiddleware handles Cross-Origin Resource Sharing
func CORSMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Max-Age", "3600")

		// Handle preflight requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

// SecurityHeadersMiddleware adds security-related headers
func SecurityHeadersMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Only add HSTS for HTTPS
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		next(w, r)
	}
}

// RateLimitMiddleware provides basic rate limiting (simple in-memory implementation)
func RateLimitMiddleware(requestsPerMinute int) func(http.HandlerFunc) http.HandlerFunc {
	// Simple in-memory rate limiter (not suitable for production)
	clients := make(map[string][]int64)

	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			clientIP := getClientIP(r)
			now := getCurrentTimeMinute()

			// Clean old entries and count current requests
			var currentRequests int
			newTimes := make([]int64, 0)

			if times, exists := clients[clientIP]; exists {
				for _, timestamp := range times {
					if now-timestamp < 60 { // Within last minute
						newTimes = append(newTimes, timestamp)
						currentRequests++
					}
				}
			}

			// Check rate limit
			if currentRequests >= requestsPerMinute {
				log.Printf("⚠️ Rate limit exceeded for %s", clientIP)
				sendErrorResponse(w, "Rate limit exceeded", "RATE_LIMIT_EXCEEDED", http.StatusTooManyRequests)
				return
			}

			// Add current request
			newTimes = append(newTimes, now)
			clients[clientIP] = newTimes

			next(w, r)
		}
	}
}

// ErrorResponseWriter wraps http.ResponseWriter to provide better error handling
type ErrorResponseWriter struct {
	http.ResponseWriter
	Request    *http.Request
	StatusCode int
}

func (w *ErrorResponseWriter) WriteHeader(statusCode int) {
	w.StatusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *ErrorResponseWriter) Write(data []byte) (int, error) {
	if w.StatusCode == 0 {
		w.StatusCode = http.StatusOK
	}
	return w.ResponseWriter.Write(data)
}

// Helper functions

// sendErrorResponse sends a standardized error response
func sendErrorResponse(w http.ResponseWriter, message, code string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	errorResp := presenters.CreateErrorResponse(
		&CustomError{Message: message, Code: code},
		code,
	)

	if err := json.NewEncoder(w).Encode(errorResp); err != nil {
		log.Printf("❌ Failed to encode error response: %v", err)
	}
}

// getClientIP extracts the client IP address from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// getCurrentTimeMinute returns current time in minutes since epoch
func getCurrentTimeMinute() int64 {
	return int64(time.Now().Unix() / 60)
}

// CustomError implements error interface for custom errors
type CustomError struct {
	Message string
	Code    string
}

func (e *CustomError) Error() string {
	return e.Message
}

// Predefined error types
var (
	ErrInvalidRequest    = &CustomError{Message: "Invalid request", Code: "INVALID_REQUEST"}
	ErrInternalServer    = &CustomError{Message: "Internal server error", Code: "INTERNAL_ERROR"}
	ErrNotFound          = &CustomError{Message: "Resource not found", Code: "NOT_FOUND"}
	ErrUnauthorized      = &CustomError{Message: "Unauthorized", Code: "UNAUTHORIZED"}
	ErrForbidden         = &CustomError{Message: "Forbidden", Code: "FORBIDDEN"}
	ErrMethodNotAllowed  = &CustomError{Message: "Method not allowed", Code: "METHOD_NOT_ALLOWED"}
	ErrRequestTooLarge   = &CustomError{Message: "Request too large", Code: "REQUEST_TOO_LARGE"}
	ErrRateLimitExceeded = &CustomError{Message: "Rate limit exceeded", Code: "RATE_LIMIT_EXCEEDED"}
)

// SendJSONError sends a JSON error response with appropriate status code
func SendJSONError(w http.ResponseWriter, err error, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	var code string
	if customErr, ok := err.(*CustomError); ok {
		code = customErr.Code
	} else {
		code = "UNKNOWN_ERROR"
	}

	errorResp := presenters.CreateErrorResponse(err, code)
	if jsonErr := json.NewEncoder(w).Encode(errorResp); jsonErr != nil {
		log.Printf("❌ Failed to encode error response: %v", jsonErr)
	}
}

// SendJSONSuccess sends a JSON success response
func SendJSONSuccess(w http.ResponseWriter, data interface{}, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	successResp := presenters.CreateSuccessResponse(message, data)
	if err := json.NewEncoder(w).Encode(successResp); err != nil {
		log.Printf("❌ Failed to encode success response: %v", err)
	}
}
