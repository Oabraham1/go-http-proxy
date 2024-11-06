package filters

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
)

// Filter defines the interface for request/response filters
type Filter interface {
	Process(*http.Request) error
	Name() string
}

// FilterChain manages a sequence of filters
type FilterChain struct {
	filters []Filter
	mu      sync.RWMutex
}

// NewFilterChain creates a new filter chain with the given filters
func NewFilterChain(filters ...Filter) *FilterChain {
	return &FilterChain{
		filters: filters,
	}
}

// Process executes all filters in the chain
func (fc *FilterChain) Process(r *http.Request) error {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	for _, filter := range fc.filters {
		if err := filter.Process(r); err != nil {
			return fmt.Errorf("filter %s failed: %w", filter.Name(), err)
		}
	}
	return nil
}

// Add appends a new filter to the chain
func (fc *FilterChain) Add(filter Filter) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.filters = append(fc.filters, filter)
}

// HeaderFilter modifies request headers
type HeaderFilter struct {
	headers map[string]string
}

// NewHeaderFilter creates a new header filter
func NewHeaderFilter(headers map[string]string) *HeaderFilter {
	return &HeaderFilter{
		headers: headers,
	}
}

// Process implements the Filter interface for HeaderFilter
func (f *HeaderFilter) Process(r *http.Request) error {
	for key, value := range f.headers {
		r.Header.Set(key, value)
	}
	return nil
}

func (f *HeaderFilter) Name() string {
	return "header"
}

// URLRewriteFilter implements URL rewriting logic
type URLRewriteFilter struct {
	rules    map[string]string
	patterns []*rewriteRule
	mu       sync.RWMutex
}

type rewriteRule struct {
	pattern *regexp.Regexp
	replace string
}

// NewURLRewriteFilter creates a new URL rewrite filter
func NewURLRewriteFilter(rules map[string]string) *URLRewriteFilter {
	f := &URLRewriteFilter{
		rules:    rules,
		patterns: make([]*rewriteRule, 0, len(rules)),
	}

	for pattern, replace := range rules {
		if !strings.HasPrefix(pattern, "/") {
			pattern = "/" + pattern
		}

		regex, err := regexp.Compile("^" + pattern + "$")
		if err != nil {
			log.Printf("Invalid rewrite pattern %q: %v", pattern, err)
			continue
		}

		f.patterns = append(f.patterns, &rewriteRule{
			pattern: regex,
			replace: replace,
		})
	}

	return f
}

// Process implements the Filter interface for URLRewriteFilter
func (f *URLRewriteFilter) Process(r *http.Request) error {
	f.mu.RLock()
	defer f.mu.RUnlock()

	path := r.URL.Path

	for _, rule := range f.patterns {
		if matches := rule.pattern.FindStringSubmatch(path); matches != nil {
			newPath := rule.replace

			// Replace captured groups
			for i, match := range matches {
				if i > 0 {
					placeholder := fmt.Sprintf("$%d", i)
					newPath = strings.Replace(newPath, placeholder, match, -1)
				}
			}

			// Handle query parameters
			if strings.Contains(newPath, "?") {
				parts := strings.SplitN(newPath, "?", 2)
				newPath = parts[0]

				newQuery, err := url.ParseQuery(parts[1])
				if err != nil {
					return fmt.Errorf("invalid query parameters in rewrite rule: %v", err)
				}

				// Merge with existing query parameters
				originalQuery := r.URL.Query()
				for key, values := range newQuery {
					originalQuery[key] = values
				}

				r.URL.RawQuery = originalQuery.Encode()
			}

			r.URL.Path = newPath
			return nil
		}
	}

	return nil
}

func (f *URLRewriteFilter) Name() string {
	return "urlrewrite"
}

// PathFilter filters requests based on path patterns
type PathFilter struct {
	patterns []*regexp.Regexp
	mu       sync.RWMutex
}

// NewPathFilter creates a new path filter
func NewPathFilter(patterns []string) *PathFilter {
	f := &PathFilter{
		patterns: make([]*regexp.Regexp, 0, len(patterns)),
	}

	for _, pattern := range patterns {
		regex, err := regexp.Compile(pattern)
		if err != nil {
			log.Printf("Invalid path pattern %q: %v", pattern, err)
			continue
		}
		f.patterns = append(f.patterns, regex)
	}

	return f
}

// Process implements the Filter interface for PathFilter
func (f *PathFilter) Process(r *http.Request) error {
	f.mu.RLock()
	defer f.mu.RUnlock()

	path := r.URL.Path
	for _, pattern := range f.patterns {
		if pattern.MatchString(path) {
			return nil
		}
	}

	return fmt.Errorf("path %s not allowed", path)
}

func (f *PathFilter) Name() string {
	return "path"
}

// MethodFilter filters requests based on HTTP methods
type MethodFilter struct {
	allowedMethods map[string]bool
}

// NewMethodFilter creates a new method filter
func NewMethodFilter(methods []string) *MethodFilter {
	allowed := make(map[string]bool)
	for _, method := range methods {
		allowed[strings.ToUpper(method)] = true
	}

	return &MethodFilter{
		allowedMethods: allowed,
	}
}

// Process implements the Filter interface for MethodFilter
func (f *MethodFilter) Process(r *http.Request) error {
	if !f.allowedMethods[r.Method] {
		return fmt.Errorf("method %s not allowed", r.Method)
	}
	return nil
}

func (f *MethodFilter) Name() string {
	return "method"
}

// QueryFilter modifies query parameters
type QueryFilter struct {
	params map[string]string
}

// NewQueryFilter creates a new query parameter filter
func NewQueryFilter(params map[string]string) *QueryFilter {
	return &QueryFilter{
		params: params,
	}
}

// Process implements the Filter interface for QueryFilter
func (f *QueryFilter) Process(r *http.Request) error {
	query := r.URL.Query()
	for key, value := range f.params {
		query.Set(key, value)
	}
	r.URL.RawQuery = query.Encode()
	return nil
}

func (f *QueryFilter) Name() string {
	return "query"
}

// CompositeFilter combines multiple filters with AND logic
type CompositeFilter struct {
	filters []Filter
}

// NewCompositeFilter creates a new composite filter
func NewCompositeFilter(filters ...Filter) *CompositeFilter {
	return &CompositeFilter{
		filters: filters,
	}
}

// Process implements the Filter interface for CompositeFilter
func (f *CompositeFilter) Process(r *http.Request) error {
	for _, filter := range f.filters {
		if err := filter.Process(r); err != nil {
			return err
		}
	}
	return nil
}

func (f *CompositeFilter) Name() string {
	return "composite"
}

// FilterError represents a filter processing error
type FilterError struct {
	Filter  string
	Message string
	Err     error
}

func (e *FilterError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s filter error: %s: %v", e.Filter, e.Message, e.Err)
	}
	return fmt.Sprintf("%s filter error: %s", e.Filter, e.Message)
}

func NewFilterError(filter, message string, err error) *FilterError {
	return &FilterError{
		Filter:  filter,
		Message: message,
		Err:     err,
	}
}
