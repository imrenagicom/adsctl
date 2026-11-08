// Package graph provides a bounded, credential-redacting Meta Graph client.
package graph

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"adsctl/internal/output"
)

type Client struct {
	BaseURL, Version, Token, AppSecret string
	HTTP                               *http.Client
	RequestTimeout, Backoff            time.Duration
	Log                                io.Writer
	pacing                             pacer
}
type Response struct {
	Value map[string]any
	Usage map[string]any
}

func New(token, version string) *Client {
	return &Client{BaseURL: "https://graph.facebook.com", Version: version, Token: token, HTTP: &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}, RequestTimeout: 30 * time.Second, Backoff: 200 * time.Millisecond}
}
func (c *Client) Redact(s string) string {
	for _, v := range []string{c.Token, c.AppSecret} {
		if v != "" {
			s = strings.ReplaceAll(s, v, "[REDACTED]")
		}
	}
	return s
}
func (c *Client) Request(ctx context.Context, method, path string, params url.Values) (Response, error) {
	if strings.ContainsAny(path, "?#\\") || strings.Contains(path, "..") || strings.Contains(path, ":") {
		return Response{}, output.Fail("USAGE", "Invalid Graph resource path.", 2)
	}
	params = clone(params)
	if c.AppSecret != "" {
		h := hmac.New(sha256.New, []byte(c.AppSecret))
		_, _ = h.Write([]byte(c.Token))
		params.Set("appsecret_proof", hex.EncodeToString(h.Sum(nil)))
	}
	for attempt := 0; ; attempt++ {
		if err := c.pace(ctx); err != nil {
			return Response{}, err
		}
		r, err := c.once(ctx, method, path, params)
		if err == nil {
			return r, nil
		}
		e := output.AsError(err)
		if method != "GET" || !e.Retryable || attempt >= 2 {
			return r, err
		}
		timer := time.NewTimer(c.Backoff * time.Duration(1<<attempt))
		select {
		case <-ctx.Done():
			timer.Stop()
			return r, output.Fail("TIMEOUT", "Operation cancelled or deadline exceeded.", 7)
		case <-timer.C:
		}
	}
}
func clone(v url.Values) url.Values {
	out := url.Values{}
	for k, vs := range v {
		out[k] = append([]string(nil), vs...)
	}
	return out
}
func (c *Client) once(ctx context.Context, method, path string, params url.Values) (Response, error) {
	base, err := url.Parse(c.BaseURL)
	if err != nil || base.Host == "" {
		return Response{}, output.Fail("CONFIG", "Invalid Graph transport base URL.", 2)
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/" + c.Version + "/" + strings.TrimLeft(path, "/")
	var body io.Reader
	if method == "GET" {
		base.RawQuery = params.Encode()
	} else {
		body = strings.NewReader(params.Encode())
	}
	requestCtx, cancel := context.WithTimeout(ctx, c.RequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, method, base.String(), body)
	if err != nil {
		return Response{}, output.Fail("INTERNAL", "Cannot construct Graph request.", 1)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("User-Agent", "adsctl/0.1")
	if method != "GET" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if c.Log != nil {
		_, _ = fmt.Fprintf(c.Log, "%s %s/%s (attempt bounded; parameters redacted)\n", method, c.Version, path)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		if method != "GET" {
			return Response{}, output.Fail("MUTATION_UNKNOWN", "Request interrupted; remote outcome is unknown. Reconcile before retrying.", 10)
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return Response{}, output.Fail("TIMEOUT", "Operation cancelled or deadline exceeded.", 7)
		}
		e := output.Fail("NETWORK", "Graph request failed.", 8)
		e.Retryable = true
		return Response{}, e
	}
	defer func() { _ = resp.Body.Close() }()
	return c.readResponse(method, resp)
}
func (c *Client) readResponse(method string, resp *http.Response) (Response, error) {
	c.observe(resp.Header)
	b, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024+1))
	if err != nil || len(b) > 32*1024*1024 {
		if method != "GET" {
			return Response{}, output.Fail("MUTATION_UNKNOWN", "Mutation acknowledgment could not be read; reconcile before retrying.", 10)
		}
		return Response{}, output.Fail("RESPONSE_INVALID", "Graph response could not be read within 32 MiB.", 8)
	}
	b = []byte(c.Redact(string(b)))
	var value map[string]any
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()
	decodeErr := dec.Decode(&value)
	if decodeErr == nil {
		var extra any
		if e := dec.Decode(&extra); !errors.Is(e, io.EOF) {
			decodeErr = fmt.Errorf("extra JSON content")
		}
	}
	if value != nil {
		c.sanitize(value)
	}
	r := Response{Value: value, Usage: map[string]any{}}
	for _, key := range []string{"X-App-Usage", "X-Ad-Account-Usage", "X-Business-Use-Case-Usage", "X-Fb-Ads-Insights-Throttle", "X-Fb-Ads-Insights-Reach-Throttle", "Facebook-Api-Version"} {
		if v := resp.Header.Get(key); v != "" {
			r.Usage[strings.ToLower(key)] = c.Redact(v)
		}
	}
	if p, ok := value["error"].(map[string]any); ok {
		e := providerError(p, method)
		e.HTTPStatus = resp.StatusCode
		if method != "GET" && resp.StatusCode >= 500 {
			e.Code, e.Exit = "MUTATION_UNKNOWN", 10
		}
		return r, e
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		e := output.Fail("PROVIDER", fmt.Sprintf("Graph returned HTTP %d.", resp.StatusCode), 8)
		e.HTTPStatus = resp.StatusCode
		// Upload hosts (rupload) explain failures in debug_info instead of error.
		if debug, ok := value["debug_info"].(map[string]any); ok {
			if reason := debugReason(debug); reason != "" {
				e.Message = fmt.Sprintf("Graph returned HTTP %d: %s.", resp.StatusCode, reason)
			}
			e.Provider = debug
		}
		if resp.StatusCode == http.StatusRequestEntityTooLarge {
			e.Message = "Graph returned HTTP 413: the request body was rejected as too large."
			e.SuggestedAction = "Reduce the upload copy or use meta videos upload --mode resumable."
		}
		e.Retryable = method == "GET" && (resp.StatusCode == 429 || resp.StatusCode >= 500)
		if resp.StatusCode == 429 {
			e.Code = "THROTTLED"
			e.Exit = 6
		}
		if method != "GET" && resp.StatusCode >= 500 {
			e.Code = "MUTATION_UNKNOWN"
			e.Exit = 10
			e.SuggestedAction = "Reconcile the existing mutation before retrying."
		}
		return r, e
	}
	if decodeErr != nil || value == nil {
		if method != "GET" {
			return r, output.Fail("MUTATION_UNKNOWN", "Mutation returned an unreadable acknowledgment; reconcile before retrying.", 10)
		}
		return r, output.Fail("RESPONSE_INVALID", "Expected a Graph JSON object.", 8)
	}
	return r, nil
}

// debugReason renders an upload host's debug_info as "Type: message".
func debugReason(debug map[string]any) string {
	t, _ := debug["type"].(string)
	m, _ := debug["message"].(string)
	m = strings.TrimSuffix(strings.TrimSpace(m), ".")
	switch {
	case t != "" && m != "":
		return t + ": " + m
	case m != "":
		return m
	default:
		return t
	}
}

func providerError(p map[string]any, method string) *output.Error {
	code := fmt.Sprint(p["code"])
	e := output.Fail("PROVIDER", fmt.Sprint(p["message"]), 8)
	e.Provider = p
	switch code {
	case "190":
		e.Code = "AUTH_INVALID"
		e.Exit = 3
	case "10", "200", "294":
		e.Code = "ACCESS_DENIED"
		e.Exit = 4
	case "100":
		e.Code = "VALIDATION"
		e.Exit = 5
	case "4", "17", "32", "613", "80000", "80004":
		e.Code = "THROTTLED"
		e.Exit = 6
	}
	transient, _ := p["is_transient"].(bool)
	e.Retryable = method == "GET" && (transient || e.Exit == 6)
	return e
}
func Form(payload map[string]any) (url.Values, error) {
	v := url.Values{}
	for k, x := range payload {
		switch y := x.(type) {
		case string:
			v.Set(k, y)
		case json.Number:
			v.Set(k, string(y))
		default:
			b, err := json.Marshal(x)
			if err != nil {
				return nil, err
			}
			v.Set(k, string(b))
		}
	}
	return v, nil
}
