package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kncept/guacamole/ai"
)

// maxFetchBytes caps how much of a response body http_fetch returns, so a
// huge page cannot flood the conversation.
const maxFetchBytes = 64 * 1024

// fetchTimeout caps one http_fetch request, including redirects.
const fetchTimeout = 30 * time.Second

// userAgent is http_fetch's default User-Agent, unless the caller sets one.
const userAgent = "guacamole-http_fetch/1.0"

// Web returns the web tools: http_fetch.
func Web(checker AccessChecker) []ai.Tool {
	return []ai.Tool{HTTPFetch(checker)}
}

// HTTPFetch returns a tool that fetches a URL over HTTP(S), like a minimal
// curl. The request's domain (and every redirect target) is checked against
// the web permission category first.
func HTTPFetch(checker AccessChecker) ai.Tool {
	return ai.Tool{
		Name: "http_fetch",
		Description: "Fetch a URL over HTTP(S) and return the response status and body, like a minimal curl. " +
			"Supports a method, extra headers and a request body. The target domain must be allowed by the web permissions.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "Absolute http:// or https:// URL to fetch",
				},
				"method": map[string]any{
					"type":        "string",
					"description": "HTTP method to use (default: GET)",
				},
				"headers": map[string]any{
					"type":                 "object",
					"description":          "Extra request headers, as name/value string pairs",
					"additionalProperties": map[string]any{"type": "string"},
				},
				"body": map[string]any{
					"type":        "string",
					"description": "Request body to send (mainly for POST/PUT)",
				},
			},
			"required": []string{"url"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			var a struct {
				URL     string            `json:"url"`
				Method  string            `json:"method"`
				Headers map[string]string `json:"headers"`
				Body    string            `json:"body"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return "", err
			}
			if a.URL == "" {
				return "", errors.New("url is required")
			}
			target, err := url.Parse(a.URL)
			if err != nil {
				return "", fmt.Errorf("invalid url: %w", err)
			}
			if target.Scheme != "http" && target.Scheme != "https" {
				return "", fmt.Errorf("unsupported url scheme %q: only http and https are allowed", target.Scheme)
			}
			if err := checkWebAccess(checker, target.Hostname()); err != nil {
				return "", err
			}

			method := strings.ToUpper(strings.TrimSpace(a.Method))
			if method == "" {
				method = http.MethodGet
			}
			var body io.Reader
			if a.Body != "" {
				body = strings.NewReader(a.Body)
			}
			req, err := http.NewRequestWithContext(ctx, method, target.String(), body)
			if err != nil {
				return "", err
			}
			for name, value := range a.Headers {
				req.Header.Set(name, value)
			}
			if req.Header.Get("User-Agent") == "" {
				req.Header.Set("User-Agent", userAgent)
			}

			client := &http.Client{
				Timeout: fetchTimeout,
				// Redirect targets are fetched domains too, so each hop is
				// checked against the web permissions as well.
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					if len(via) >= 10 {
						return errors.New("stopped after 10 redirects")
					}
					return checkWebAccess(checker, req.URL.Hostname())
				},
			}
			resp, err := client.Do(req)
			if err != nil {
				return "", err
			}
			defer resp.Body.Close()

			data, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes+1))
			if err != nil {
				return "", err
			}
			truncated := len(data) > maxFetchBytes
			if truncated {
				data = data[:maxFetchBytes]
			}

			var b strings.Builder
			// resp.Status already reads e.g. "200 OK".
			fmt.Fprintf(&b, "HTTP %s\n\n", resp.Status)
			if len(data) == 0 {
				b.WriteString("(empty body)")
			} else {
				b.Write(data)
				if truncated {
					fmt.Fprintf(&b, "\n... (truncated: body is at least %d bytes)", maxFetchBytes)
				}
			}
			return b.String(), nil
		},
	}
}

// checkWebAccess applies the web permission category (per-domain
// allow/ask/deny) to a request's domain.
func checkWebAccess(checker AccessChecker, domain string) error {
	if checker == nil {
		return nil
	}
	if domain == "" {
		return errors.New("url has no domain")
	}
	allowed, err := checker.IsAllowedDomain(domain)
	if err != nil {
		return err
	}
	if !allowed {
		return fmt.Errorf("web access to %s denied", domain)
	}
	return nil
}
