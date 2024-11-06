package proxy

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// LogEntry represents a structured log entry for requests/responses
type LogEntry struct {
	Timestamp    time.Time              `json:"timestamp"`
	RequestID    string                 `json:"request_id"`
	Method       string                 `json:"method"`
	Path         string                 `json:"path"`
	RemoteAddr   string                 `json:"remote_addr"`
	Duration     time.Duration          `json:"duration"`
	StatusCode   int                    `json:"status_code"`
	ResponseSize int64                  `json:"response_size"`
	CacheHit     bool                   `json:"cache_hit"`
	Error        string                 `json:"error,omitempty"`
	Service      string                 `json:"service,omitempty"`
	Headers      map[string]string      `json:"headers,omitempty"`
	ExtraData    map[string]interface{} `json:"extra_data,omitempty"`
}

// loggedResponseWriter wraps http.ResponseWriter to capture response data
type loggedResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	responseSize int64
}

func (w *loggedResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *loggedResponseWriter) Write(b []byte) (int, error) {
	size, err := w.ResponseWriter.Write(b)
	w.responseSize += int64(size)
	return size, err
}

func (p *Proxy) logRequest(start time.Time, w http.ResponseWriter, r *http.Request, service string, cacheHit bool, err error) {
	duration := time.Since(start)

	// Get response data if available
	lw, ok := w.(*loggedResponseWriter)
	statusCode := http.StatusOK
	responseSize := int64(0)
	if ok {
		statusCode = lw.statusCode
		responseSize = lw.responseSize
	}

	// Create request headers map, filtering sensitive data
	headers := make(map[string]string)
	sensitiveHeaders := map[string]bool{
		"Authorization":  true,
		"Cookie":         true,
		"Set-Cookie":     true,
		"X-API-Key":      true,
		"X-Auth-Token":   true,
		"X-Access-Token": true,
	}

	for name, values := range r.Header {
		if !sensitiveHeaders[name] {
			headers[name] = strings.Join(values, ", ")
		}
	}

	// Create the log entry
	entry := LogEntry{
		Timestamp:    start,
		RequestID:    r.Header.Get("X-Request-ID"),
		Method:       r.Method,
		Path:         r.URL.Path,
		RemoteAddr:   r.RemoteAddr,
		Duration:     duration,
		StatusCode:   statusCode,
		ResponseSize: responseSize,
		CacheHit:     cacheHit,
		Service:      service,
		Headers:      headers,
		ExtraData:    make(map[string]interface{}),
	}

	// Add error information if present
	if err != nil {
		entry.Error = err.Error()
		entry.ExtraData["error_type"] = fmt.Sprintf("%T", err)
	}

	// Add circuit breaker status if available
	if breaker, exists := p.breakers[service]; exists {
		entry.ExtraData["circuit_breaker_state"] = breaker.GetState()
	}

	// Add cache metrics
	entry.ExtraData["cache_hits"] = p.metrics.cacheHits.Load()
	entry.ExtraData["cache_misses"] = p.metrics.cacheMisses.Load()

	// Log the entry
	p.writeLog(entry)
}

func (p *Proxy) wrapResponseWriter(w http.ResponseWriter) *loggedResponseWriter {
	return &loggedResponseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK, // Default status code
	}
}

func (p *Proxy) writeLog(entry LogEntry) {
	// Convert the entry to JSON
	jsonData, err := json.Marshal(entry)
	if err != nil {
		log.Printf("Error marshaling log entry: %v", err)
		return
	}

	// Write to log
	log.Println(string(jsonData))
}
