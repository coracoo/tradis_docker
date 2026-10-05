package aihttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var sharedTransports sync.Map

type ErrorKind string

const (
	ErrorInvalidRequest ErrorKind = "invalid_request"
	ErrorAuthentication ErrorKind = "authentication"
	ErrorPermission     ErrorKind = "permission"
	ErrorRateLimit      ErrorKind = "rate_limit"
	ErrorOverloaded     ErrorKind = "overloaded"
	ErrorTimeout        ErrorKind = "timeout"
	ErrorCanceled       ErrorKind = "canceled"
	ErrorNetwork        ErrorKind = "network"
	ErrorProvider       ErrorKind = "provider"
)

type Error struct {
	Kind       ErrorKind
	StatusCode int
	Attempts   int
	TraceID    string
	Body       string
	Cause      error
}

func (e *Error) Error() string {
	if e == nil {
		return "AI request failed"
	}
	message := "AI request failed"
	if e.StatusCode > 0 {
		message = fmt.Sprintf("AI provider returned HTTP %d", e.StatusCode)
	}
	if e.Cause != nil {
		message += ": " + e.Cause.Error()
	}
	return message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type Policy struct {
	TotalTimeout          time.Duration
	ResponseHeaderTimeout time.Duration
	MaxRetries            int
	RetryDelay            time.Duration
}

func DiagnosticPolicy() Policy {
	return Policy{
		TotalTimeout:          15 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
	}
}

func UtilityPolicy() Policy {
	return Policy{
		TotalTimeout:          90 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		MaxRetries:            1,
		RetryDelay:            350 * time.Millisecond,
	}
}

func AgentPolicy() Policy {
	return Policy{
		TotalTimeout:          180 * time.Second,
		ResponseHeaderTimeout: 90 * time.Second,
		MaxRetries:            1,
		RetryDelay:            350 * time.Millisecond,
	}
}

// AgentHarnessPolicy backs the ADK Harness model rounds. Long deployments
// append large tool evidence before the final answer, so the stream needs a
// wider total/header budget than the generic agent policy.
func AgentHarnessPolicy() Policy {
	return Policy{
		TotalTimeout:          300 * time.Second,
		ResponseHeaderTimeout: 120 * time.Second,
		MaxRetries:            2,
		RetryDelay:            500 * time.Millisecond,
	}
}

type Result struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	TraceID    string
	Attempts   int
	Duration   time.Duration
}

type StreamResult struct {
	Response *http.Response
	TraceID  string
	Attempts int
	Duration time.Duration
	cancel   context.CancelFunc
}

func (r *StreamResult) Close() error {
	if r == nil {
		return nil
	}
	var err error
	if r.Response != nil && r.Response.Body != nil {
		err = r.Response.Body.Close()
	}
	if r.cancel != nil {
		r.cancel()
	}
	return err
}

type Client struct {
	policy     Policy
	httpClient *http.Client
}

func New(policy Policy) *Client {
	if policy.TotalTimeout <= 0 {
		policy.TotalTimeout = 90 * time.Second
	}
	if policy.ResponseHeaderTimeout <= 0 {
		policy.ResponseHeaderTimeout = policy.TotalTimeout
	}
	if policy.MaxRetries < 0 {
		policy.MaxRetries = 0
	}
	if policy.MaxRetries > 1 {
		policy.MaxRetries = 1
	}
	transport := sharedTransport(policy.ResponseHeaderTimeout)
	return &Client{
		policy: policy,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   0,
		},
	}
}

func sharedTransport(responseHeaderTimeout time.Duration) *http.Transport {
	if existing, ok := sharedTransports.Load(responseHeaderTimeout); ok {
		return existing.(*http.Transport)
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          50,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: responseHeaderTimeout,
	}
	actual, _ := sharedTransports.LoadOrStore(responseHeaderTimeout, transport)
	if existing, ok := actual.(*http.Transport); ok {
		return existing
	}
	return transport
}

func ChatCompletionsURL(rawBaseURL string) (string, error) {
	baseURL, err := normalizeBaseURL(rawBaseURL)
	if err != nil {
		return "", err
	}
	return appendAPIPath(baseURL, "/chat/completions")
}

func ModelsURL(rawBaseURL string) (string, error) {
	baseURL, err := normalizeBaseURL(rawBaseURL)
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse AI base URL: %w", err)
	}
	path := strings.TrimRight(parsed.Path, "/")
	const chatCompletionsPath = "/chat/completions"
	if strings.HasSuffix(strings.ToLower(path), chatCompletionsPath) {
		parsed.Path = path[:len(path)-len(chatCompletionsPath)] + "/models"
		return parsed.String(), nil
	}
	return appendAPIPath(baseURL, "/models")
}

func appendAPIPath(baseURL, suffix string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse AI base URL: %w", err)
	}
	path := strings.TrimRight(parsed.Path, "/")
	if !strings.HasSuffix(strings.ToLower(path), strings.ToLower(suffix)) {
		path += suffix
	}
	parsed.Path = path
	return parsed.String(), nil
}

func normalizeBaseURL(rawBaseURL string) (string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(rawBaseURL), "/")
	if baseURL == "" {
		return "", errors.New("AI base URL is empty")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse AI base URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("AI base URL must use http or https")
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", errors.New("AI base URL host is empty")
	}
	return baseURL, nil
}

func (c *Client) ChatCompletions(ctx context.Context, baseURL string, apiKey string, payload any) (*Result, error) {
	endpoint, err := ChatCompletionsURL(baseURL)
	if err != nil {
		return nil, &Error{Kind: ErrorInvalidRequest, Cause: err}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, &Error{Kind: ErrorInvalidRequest, Cause: err}
	}
	return c.doJSON(ctx, http.MethodPost, endpoint, apiKey, body)
}

func (c *Client) Models(ctx context.Context, baseURL string, apiKey string) (*Result, error) {
	endpoint, err := ModelsURL(baseURL)
	if err != nil {
		return nil, &Error{Kind: ErrorInvalidRequest, Cause: err}
	}
	return c.doJSON(ctx, http.MethodGet, endpoint, apiKey, nil)
}

func (c *Client) OpenChatCompletions(parent context.Context, baseURL string, apiKey string, payload any) (*StreamResult, error) {
	endpoint, err := ChatCompletionsURL(baseURL)
	if err != nil {
		return nil, &Error{Kind: ErrorInvalidRequest, Cause: err}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, &Error{Kind: ErrorInvalidRequest, Cause: err}
	}

	ctx, cancel := context.WithTimeout(parent, c.policy.TotalTimeout)
	started := time.Now()
	maxAttempts := c.policy.MaxRetries + 1
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if requestErr != nil {
			cancel()
			return nil, &Error{Kind: ErrorInvalidRequest, Attempts: attempt, Cause: requestErr}
		}
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Content-Type", "application/json")
		if strings.TrimSpace(apiKey) != "" {
			req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
		}

		resp, requestErr := c.httpClient.Do(req)
		if requestErr != nil {
			providerErr := classifyRequestError(ctx, requestErr, attempt)
			if attempt < maxAttempts && shouldRetryError(ctx, providerErr) && waitForRetry(ctx, c.policy.RetryDelay) {
				continue
			}
			cancel()
			return nil, providerErr
		}

		traceID := firstHeader(resp.Header, "x-siliconcloud-trace-id", "x-request-id")
		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			return &StreamResult{
				Response: resp,
				TraceID:  traceID,
				Attempts: attempt,
				Duration: time.Since(started),
				cancel:   cancel,
			}, nil
		}

		responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
		_ = resp.Body.Close()
		providerErr := &Error{
			Kind:       classifyStatus(resp.StatusCode),
			StatusCode: resp.StatusCode,
			Attempts:   attempt,
			TraceID:    traceID,
			Body:       compactBody(responseBody),
			Cause:      readErr,
		}
		if attempt < maxAttempts && shouldRetryStatus(resp.StatusCode) && waitForRetry(ctx, c.policy.RetryDelay) {
			continue
		}
		cancel()
		return nil, providerErr
	}

	cancel()
	return nil, &Error{Kind: ErrorProvider, Attempts: maxAttempts, Cause: errors.New("AI stream request exhausted attempts")}
}

func (c *Client) doJSON(parent context.Context, method string, endpoint string, apiKey string, body []byte) (*Result, error) {
	ctx, cancel := context.WithTimeout(parent, c.policy.TotalTimeout)
	defer cancel()
	started := time.Now()
	maxAttempts := c.policy.MaxRetries + 1

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, &Error{Kind: ErrorInvalidRequest, Attempts: attempt, Cause: err}
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")
		if strings.TrimSpace(apiKey) != "" {
			req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
		}

		resp, requestErr := c.httpClient.Do(req)
		if requestErr != nil {
			providerErr := classifyRequestError(ctx, requestErr, attempt)
			if attempt < maxAttempts && shouldRetryError(ctx, providerErr) {
				if !waitForRetry(ctx, c.policy.RetryDelay) {
					providerErr.Kind = ErrorTimeout
					providerErr.Cause = ctx.Err()
					return nil, providerErr
				}
				continue
			}
			return nil, providerErr
		}

		responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
		_ = resp.Body.Close()
		traceID := firstHeader(resp.Header, "x-siliconcloud-trace-id", "x-request-id")
		if readErr != nil {
			providerErr := &Error{Kind: ErrorNetwork, StatusCode: resp.StatusCode, Attempts: attempt, TraceID: traceID, Cause: readErr}
			if attempt < maxAttempts && shouldRetryError(ctx, providerErr) {
				if !waitForRetry(ctx, c.policy.RetryDelay) {
					providerErr.Kind = ErrorTimeout
					providerErr.Cause = ctx.Err()
					return nil, providerErr
				}
				continue
			}
			return nil, providerErr
		}

		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			return &Result{
				StatusCode: resp.StatusCode,
				Header:     resp.Header.Clone(),
				Body:       responseBody,
				TraceID:    traceID,
				Attempts:   attempt,
				Duration:   time.Since(started),
			}, nil
		}

		providerErr := &Error{
			Kind:       classifyStatus(resp.StatusCode),
			StatusCode: resp.StatusCode,
			Attempts:   attempt,
			TraceID:    traceID,
			Body:       compactBody(responseBody),
		}
		if attempt < maxAttempts && shouldRetryStatus(resp.StatusCode) {
			if !waitForRetry(ctx, c.policy.RetryDelay) {
				providerErr.Kind = ErrorTimeout
				providerErr.Cause = ctx.Err()
				return nil, providerErr
			}
			continue
		}
		return nil, providerErr
	}

	return nil, &Error{Kind: ErrorProvider, Attempts: maxAttempts, Cause: errors.New("AI request exhausted attempts")}
}

func classifyRequestError(ctx context.Context, err error, attempts int) *Error {
	kind := ErrorNetwork
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		kind = ErrorCanceled
	} else if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		kind = ErrorTimeout
	} else {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			kind = ErrorTimeout
		}
	}
	return &Error{Kind: kind, Attempts: attempts, Cause: err}
}

func classifyStatus(statusCode int) ErrorKind {
	switch statusCode {
	case http.StatusBadRequest:
		return ErrorInvalidRequest
	case http.StatusUnauthorized:
		return ErrorAuthentication
	case http.StatusForbidden:
		return ErrorPermission
	case http.StatusRequestTimeout:
		return ErrorTimeout
	case http.StatusTooManyRequests:
		return ErrorRateLimit
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return ErrorOverloaded
	default:
		return ErrorProvider
	}
}

func shouldRetryStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests ||
		statusCode == http.StatusBadGateway ||
		statusCode == http.StatusServiceUnavailable ||
		statusCode == http.StatusGatewayTimeout
}

func shouldRetryError(ctx context.Context, err *Error) bool {
	if ctx.Err() != nil || err == nil {
		return false
	}
	return err.Kind == ErrorTimeout || err.Kind == ErrorNetwork
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func firstHeader(header http.Header, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(header.Get(name)); value != "" {
			return value
		}
	}
	return ""
}

func compactBody(body []byte) string {
	value := strings.TrimSpace(string(body))
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	if len(value) > 1024 {
		value = value[:1024]
	}
	return value
}
