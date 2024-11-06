package filters

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFilterChain(t *testing.T) {
	tests := []struct {
		name          string
		filters       []Filter
		request       *http.Request
		wantHeaders   map[string]string
		wantError     bool
		wantErrorType string
	}{
		{
			name: "single header filter",
			filters: []Filter{
				NewHeaderFilter(map[string]string{
					"X-Test": "value",
				}),
			},
			request: httptest.NewRequest("GET", "/test", nil),
			wantHeaders: map[string]string{
				"X-Test": "value",
			},
		},
		{
			name: "multiple filters",
			filters: []Filter{
				NewHeaderFilter(map[string]string{
					"X-First": "1",
				}),
				NewHeaderFilter(map[string]string{
					"X-Second": "2",
				}),
			},
			request: httptest.NewRequest("GET", "/test", nil),
			wantHeaders: map[string]string{
				"X-First":  "1",
				"X-Second": "2",
			},
		},
		{
			name: "url rewrite filter",
			filters: []Filter{
				NewURLRewriteFilter(map[string]string{
					"/old/(.*)": "/new/$1",
				}),
			},
			request:     httptest.NewRequest("GET", "/old/path", nil),
			wantHeaders: map[string]string{},
			// URL should be rewritten to /new/path
		},
		{
			name: "filter with error",
			filters: []Filter{
				newTestFilter(true), // Filter that returns error
			},
			request:       httptest.NewRequest("GET", "/test", nil),
			wantError:     true,
			wantErrorType: "TestError",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := NewFilterChain(tt.filters...)
			err := chain.Process(tt.request)

			// Check error condition
			if (err != nil) != tt.wantError {
				t.Errorf("got error = %v, want error = %v", err != nil, tt.wantError)
			}
			if err != nil && tt.wantErrorType != "" {
				if !strings.Contains(err.Error(), tt.wantErrorType) {
					t.Errorf("got error type %v, want error type %v", err, tt.wantErrorType)
				}
			}

			// Check headers
			for header, want := range tt.wantHeaders {
				if got := tt.request.Header.Get(header); got != want {
					t.Errorf("header %s = %q; want %q", header, got, want)
				}
			}
		})
	}
}

func TestURLRewriteFilter(t *testing.T) {
	tests := []struct {
		name      string
		rules     map[string]string
		path      string
		wantPath  string
		wantError bool
	}{
		{
			name: "simple rewrite",
			rules: map[string]string{
				"/old": "/new",
			},
			path:     "/old",
			wantPath: "/new",
		},
		{
			name: "pattern rewrite",
			rules: map[string]string{
				"/api/v1/(.*)": "/api/v2/$1",
			},
			path:     "/api/v1/users",
			wantPath: "/api/v2/users",
		},
		{
			name: "multiple patterns",
			rules: map[string]string{
				"/api/v1/users/(.*)": "/users/$1",
				"/api/v1/posts/(.*)": "/posts/$1",
			},
			path:     "/api/v1/users/123",
			wantPath: "/users/123",
		},
		{
			name: "no matching rule",
			rules: map[string]string{
				"/api/(.*)": "/v1/api/$1",
			},
			path:     "/other/path",
			wantPath: "/other/path", // Should remain unchanged
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := NewURLRewriteFilter(tt.rules)
			req := httptest.NewRequest("GET", tt.path, nil)

			err := filter.Process(req)

			if (err != nil) != tt.wantError {
				t.Errorf("got error = %v, want error = %v", err != nil, tt.wantError)
			}

			if got := req.URL.Path; got != tt.wantPath {
				t.Errorf("path = %q; want %q", got, tt.wantPath)
			}
		})
	}
}

func TestHeaderFilter(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		existing map[string]string
		want     map[string]string
	}{
		{
			name: "add new headers",
			headers: map[string]string{
				"X-New": "value",
			},
			want: map[string]string{
				"X-New": "value",
			},
		},
		{
			name: "override existing headers",
			headers: map[string]string{
				"X-Existing": "new-value",
			},
			existing: map[string]string{
				"X-Existing": "old-value",
			},
			want: map[string]string{
				"X-Existing": "new-value",
			},
		},
		{
			name: "multiple headers",
			headers: map[string]string{
				"X-First":  "1",
				"X-Second": "2",
			},
			existing: map[string]string{
				"X-Old": "old",
			},
			want: map[string]string{
				"X-First":  "1",
				"X-Second": "2",
				"X-Old":    "old",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := NewHeaderFilter(tt.headers)
			req := httptest.NewRequest("GET", "/test", nil)

			// Set existing headers
			for k, v := range tt.existing {
				req.Header.Set(k, v)
			}

			err := filter.Process(req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Check all expected headers
			for k, want := range tt.want {
				if got := req.Header.Get(k); got != want {
					t.Errorf("header %s = %q; want %q", k, got, want)
				}
			}
		})
	}
}

func TestFilterChainOrder(t *testing.T) {
	var order []string

	filter1 := &orderTestFilter{name: "first", order: &order}
	filter2 := &orderTestFilter{name: "second", order: &order}
	filter3 := &orderTestFilter{name: "third", order: &order}

	chain := NewFilterChain(filter1, filter2, filter3)
	req := httptest.NewRequest("GET", "/test", nil)

	err := chain.Process(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(order) != 3 {
		t.Errorf("got %d filters executed, want 3", len(order))
	}

	wantOrder := []string{"first", "second", "third"}
	for i, want := range wantOrder {
		if i >= len(order) {
			t.Errorf("missing filter execution at position %d", i)
			continue
		}
		if got := order[i]; got != want {
			t.Errorf("filter at position %d = %q; want %q", i, got, want)
		}
	}
}

// Helper test types
type testFilter struct {
	shouldError bool
}

func newTestFilter(shouldError bool) Filter {
	return &testFilter{shouldError: shouldError}
}

func (f *testFilter) Process(r *http.Request) error {
	if f.shouldError {
		return fmt.Errorf("TestError: intentional error")
	}
	return nil
}

func (f *testFilter) Name() string {
	return "test"
}

type orderTestFilter struct {
	name  string
	order *[]string
}

func (f *orderTestFilter) Process(r *http.Request) error {
	*f.order = append(*f.order, f.name)
	return nil
}

func (f *orderTestFilter) Name() string {
	return f.name
}

func BenchmarkFilterChain(b *testing.B) {
	benchmarks := []struct {
		name    string
		filters []Filter
	}{
		{
			name: "single filter",
			filters: []Filter{
				NewHeaderFilter(map[string]string{"X-Test": "value"}),
			},
		},
		{
			name: "multiple filters",
			filters: []Filter{
				NewHeaderFilter(map[string]string{"X-First": "1"}),
				NewURLRewriteFilter(map[string]string{"/old": "/new"}),
				NewHeaderFilter(map[string]string{"X-Last": "2"}),
			},
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			chain := NewFilterChain(bm.filters...)
			req := httptest.NewRequest("GET", "/test", nil)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				chain.Process(req)
			}
		})
	}
}
