package middleware

import (
	"bytes"
	"encoding/json"
	"go-drive-duplicates/internal/interfaces/presenters"
	"io"
	"log"
	"net/http"
	"time"
)

// LoggingMiddleware logs HTTP requests and responses
func LoggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()

		// Log request
		log.Printf("🔵 %s %s - %s", r.Method, r.URL.Path, r.RemoteAddr)

		// Create response recorder to capture response
		recorder := &ResponseRecorder{
			ResponseWriter: w,
			StatusCode:     http.StatusOK,
			Body:           &bytes.Buffer{},
		}

		// Call next handler
		next(recorder, r)

		// Log response
		duration := time.Since(startTime)
		statusEmoji := getStatusEmoji(recorder.StatusCode)

		log.Printf("%s %s %s - %d - %v",
			statusEmoji, r.Method, r.URL.Path, recorder.StatusCode, duration)

		// Log errors if status code indicates an error
		if recorder.StatusCode >= 400 {
			logErrorResponse(r, recorder)
		}
	}
}

// DetailedLoggingMiddleware provides more detailed logging including request/response bodies
func DetailedLoggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()

		// Read and log request body (for POST/PUT requests)
		var requestBody []byte
		if r.Method == http.MethodPost || r.Method == http.MethodPut {
			requestBody, _ = io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewBuffer(requestBody))

			if len(requestBody) > 0 && len(requestBody) < 1000 { // Only log small bodies
				log.Printf("📤 Request Body: %s", string(requestBody))
			}
		}

		// Log request details
		log.Printf("🔵 %s %s - %s - User-Agent: %s",
			r.Method, r.URL.Path, r.RemoteAddr, r.UserAgent())

		// Create response recorder
		recorder := &ResponseRecorder{
			ResponseWriter: w,
			StatusCode:     http.StatusOK,
			Body:           &bytes.Buffer{},
		}

		// Call next handler
		next(recorder, r)

		// Log response details
		duration := time.Since(startTime)
		statusEmoji := getStatusEmoji(recorder.StatusCode)

		log.Printf("%s %s %s - %d - %v - %d bytes",
			statusEmoji, r.Method, r.URL.Path, recorder.StatusCode,
			duration, recorder.Body.Len())

		// Log response body for errors or small successful responses
		if recorder.StatusCode >= 400 || (recorder.StatusCode < 300 && recorder.Body.Len() < 500) {
			if recorder.Body.Len() > 0 {
				log.Printf("📥 Response Body: %s", recorder.Body.String())
			}
		}
	}
}

// APILoggingMiddleware logs API requests in structured format
func APILoggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()

		// Create response recorder
		recorder := &ResponseRecorder{
			ResponseWriter: w,
			StatusCode:     http.StatusOK,
			Body:           &bytes.Buffer{},
		}

		// Call next handler
		next(recorder, r)

		// Create structured log entry
		logEntry := map[string]interface{}{
			"timestamp":     startTime.Format(time.RFC3339),
			"method":        r.Method,
			"path":          r.URL.Path,
			"query":         r.URL.RawQuery,
			"remote_addr":   r.RemoteAddr,
			"user_agent":    r.UserAgent(),
			"status_code":   recorder.StatusCode,
			"duration_ms":   time.Since(startTime).Milliseconds(),
			"response_size": recorder.Body.Len(),
		}

		// Add error details if status indicates error
		if recorder.StatusCode >= 400 {
			logEntry["error"] = true
			if recorder.Body.Len() > 0 {
				var errorResp presenters.ErrorResponse
				if err := json.Unmarshal(recorder.Body.Bytes(), &errorResp); err == nil {
					logEntry["error_message"] = errorResp.Error
					logEntry["error_code"] = errorResp.Code
				}
			}
		}

		// Log as JSON
		if logData, err := json.Marshal(logEntry); err == nil {
			log.Printf("📊 API: %s", string(logData))
		}
	}
}

// ResponseRecorder captures response data for logging
type ResponseRecorder struct {
	http.ResponseWriter
	StatusCode int
	Body       *bytes.Buffer
}

func (rr *ResponseRecorder) WriteHeader(statusCode int) {
	rr.StatusCode = statusCode
	rr.ResponseWriter.WriteHeader(statusCode)
}

func (rr *ResponseRecorder) Write(data []byte) (int, error) {
	// Write to both the actual response and our buffer
	rr.Body.Write(data)
	return rr.ResponseWriter.Write(data)
}

// getStatusEmoji returns an emoji based on HTTP status code
func getStatusEmoji(statusCode int) string {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return "✅"
	case statusCode >= 300 && statusCode < 400:
		return "🔄"
	case statusCode >= 400 && statusCode < 500:
		return "⚠️"
	case statusCode >= 500:
		return "❌"
	default:
		return "📋"
	}
}

// logErrorResponse logs detailed error response information
func logErrorResponse(r *http.Request, recorder *ResponseRecorder) {
	if recorder.Body.Len() > 0 {
		log.Printf("❌ Error Response for %s %s: %s",
			r.Method, r.URL.Path, recorder.Body.String())
	}
}
