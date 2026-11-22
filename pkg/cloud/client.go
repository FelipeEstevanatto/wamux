package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/felipeestevanatto/wamux/pkg/config"
	logger_wrapper "github.com/felipeestevanatto/wamux/pkg/logger"
	"github.com/felipeestevanatto/wamux/pkg/ssrf"
)

// GraphError is Meta's error envelope plus request context. It is deliberately
// separate from the public API error: handlers decide what to expose.
type GraphError struct {
	StatusCode int
	Method     string
	URL        string
	Type       string
	Message    string
	Code       int
	Subcode    int
	UserTitle  string
	UserMsg    string
	Details    string
	TraceID    string
	RetryAfter time.Duration
	Raw        []byte
}

func (e *GraphError) Error() string {
	msg := e.Message
	if e.Details != "" {
		msg = e.Details
	}
	if e.Code != 0 {
		return fmt.Sprintf("graph api error %d.%d: %s", e.Code, e.Subcode, msg)
	}
	return fmt.Sprintf("graph api error status=%d: %s", e.StatusCode, msg)
}

func (e *GraphError) IsRetryable() bool {
	return e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= 500
}

// Client is the Graph HTTP boundary. Every method takes an explicit access
// token so one process can serve multiple tenants without mutable clients.
type Client struct {
	baseURL      string
	version      string
	httpClient   *http.Client
	logger       *logger_wrapper.LoggerManager
	limiter      *adaptiveLimiter
	mediaClient  *http.Client
	timeout      time.Duration
	mediaTimeout time.Duration
}

// ClientOption customises the Graph client.
type ClientOption func(*Client)

func WithHTTPClient(c *http.Client) ClientOption {
	return func(client *Client) {
		if c != nil {
			client.httpClient = c
			client.mediaClient = c
		}
	}
}

// NewClient creates a Graph client. The base URL/version normally come from
// config; tests point them at httptest.
func NewClient(cfg *config.Config, logger *logger_wrapper.LoggerManager, opts ...ClientOption) *Client {
	base := "https://graph.facebook.com"
	version := "v27.0"
	if cfg != nil {
		if strings.TrimSpace(cfg.CloudAPIGraphBaseURL) != "" {
			base = strings.TrimRight(strings.TrimSpace(cfg.CloudAPIGraphBaseURL), "/")
		}
		if strings.TrimSpace(cfg.CloudAPIGraphVersion) != "" {
			version = strings.TrimSpace(cfg.CloudAPIGraphVersion)
		}
	}
	transport := &http.Transport{
		Proxy:                  http.ProxyFromEnvironment,
		DialContext:            (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:    10 * time.Second,
		ResponseHeaderTimeout:  60 * time.Second,
		MaxIdleConns:           100,
		MaxIdleConnsPerHost:    20,
		IdleConnTimeout:        90 * time.Second,
		MaxResponseHeaderBytes: 1 << 20,
	}
	c := &Client{
		baseURL:      base,
		version:      version,
		httpClient:   &http.Client{Transport: transport},
		logger:       logger,
		limiter:      newAdaptiveLimiter(8),
		mediaClient:  ssrf.Default().Client(120 * time.Second),
		timeout:      30 * time.Second,
		mediaTimeout: 120 * time.Second,
	}
	if cfg != nil {
		if cfg.CloudAPIHTTPTimeout > 0 {
			c.timeout = cfg.CloudAPIHTTPTimeout
		}
		if cfg.CloudAPIMediaTimeout > 0 {
			c.mediaTimeout = cfg.CloudAPIMediaTimeout
		}
		if cfg.CloudAPIMaxHeaderBytes > 0 {
			transport.MaxResponseHeaderBytes = cfg.CloudAPIMaxHeaderBytes
		}
	}
	if cfg != nil && cfg.CloudAPIRPS > 0 {
		c.limiter = newAdaptiveLimiter(cfg.CloudAPIRPS)
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// EffectiveRPS reports the adaptive limiter's current rate for diagnostics.
func (c *Client) EffectiveRPS() float64 { return c.limiter.RPS() }

// SendMessage posts a message payload. It is intentionally single-attempt:
// re-sending a POST after a timeout can duplicate a customer message.
func (c *Client) SendMessage(ctx context.Context, token, phoneNumberID string, payload any) (SendResponse, error) {
	var out SendResponse
	err := c.doJSON(ctx, token, http.MethodPost, phoneNumberID+"/messages", payload, &out, false)
	return out, err
}

// UploadMedia uploads bytes and returns the media id.
func (c *Client) UploadMedia(ctx context.Context, token, phoneNumberID string, data []byte, mime, filename string) (string, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	fw, err := w.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := fw.Write(data); err != nil {
		return "", err
	}
	if err := w.WriteField("type", mime); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}

	var out struct {
		ID string `json:"id"`
	}
	ctx, cancel := c.withTimeout(ctx, c.mediaTimeout)
	defer cancel()
	err = c.doRaw(ctx, token, http.MethodPost, phoneNumberID+"/media", w.FormDataContentType(), body.Bytes(), &out, false)
	return out.ID, err
}

// GetMedia resolves a media id to its short-lived download URL.
func (c *Client) GetMedia(ctx context.Context, token, mediaID string) (MediaInfo, error) {
	var out MediaInfo
	err := c.doJSON(ctx, token, http.MethodGet, mediaID, nil, &out, true)
	return out, err
}

// DownloadMedia fetches a Graph-provided media URL with the access token.
func (c *Client) DownloadMedia(ctx context.Context, token, rawURL string, maxBytes int64) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid media url: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, errors.New("media url must be http(s)")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client := c.mediaClient
	if client == nil {
		client = c.httpClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, graphErrorFromResponse(resp, http.MethodGet, rawURL)
	}
	if maxBytes <= 0 {
		maxBytes = 100 << 20
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("media exceeds %d bytes", maxBytes)
	}
	return data, nil
}

// VerifyPhone returns the display name, quality and registration status.
func (c *Client) VerifyPhone(ctx context.Context, token, phoneNumberID string) (PhoneInfo, error) {
	var out PhoneInfo
	query := url.Values{}
	query.Set("fields", "display_phone_number,verified_name,quality_rating,status")
	path := phoneNumberID
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	err := c.doJSON(ctx, token, http.MethodGet, path, nil, &out, true)
	return out, err
}

// RegisterPhone registers a number with its six-digit PIN. The PIN is not
// persisted by this package.
func (c *Client) RegisterPhone(ctx context.Context, token, phoneNumberID, pin string) error {
	payload := map[string]string{"messaging_product": "whatsapp", "pin": pin}
	var out map[string]any
	return c.doJSON(ctx, token, http.MethodPost, phoneNumberID+"/register", payload, &out, false)
}

func (c *Client) DeregisterPhone(ctx context.Context, token, phoneNumberID string) error {
	var out map[string]any
	return c.doJSON(ctx, token, http.MethodPost, phoneNumberID+"/deregister", map[string]string{"messaging_product": "whatsapp"}, &out, false)
}

func (c *Client) SubscribeApp(ctx context.Context, token, wabaID string) error {
	var out map[string]any
	return c.doJSON(ctx, token, http.MethodPost, wabaID+"/subscribed_apps", map[string]any{}, &out, false)
}

// ListTemplates returns one page and the next cursor.
func (c *Client) ListTemplates(ctx context.Context, token, wabaID, after string) ([]TemplateInfo, string, error) {
	q := url.Values{}
	q.Set("fields", "id,name,status,category,language,components,rejected_reason,quality_score")
	q.Set("limit", "100")
	if after != "" {
		q.Set("after", after)
	}
	var out struct {
		Data   []TemplateInfo `json:"data"`
		Paging struct {
			Next    string `json:"next"`
			Cursors struct {
				After string `json:"after"`
			} `json:"cursors"`
		} `json:"paging"`
	}
	if err := c.doJSON(ctx, token, http.MethodGet, wabaID+"/message_templates?"+q.Encode(), nil, &out, true); err != nil {
		return nil, "", err
	}
	return out.Data, out.Paging.Cursors.After, nil
}

func (c *Client) CreateTemplate(ctx context.Context, token, wabaID string, payload any) (TemplateInfo, error) {
	var out TemplateInfo
	err := c.doJSON(ctx, token, http.MethodPost, wabaID+"/message_templates", payload, &out, false)
	return out, err
}

func (c *Client) UpdateTemplate(ctx context.Context, token, templateID string, payload any) (TemplateInfo, error) {
	var out TemplateInfo
	err := c.doJSON(ctx, token, http.MethodPost, templateID, payload, &out, false)
	return out, err
}

func (c *Client) DeleteTemplate(ctx context.Context, token, wabaID, name string) error {
	q := url.Values{}
	q.Set("name", name)
	var out map[string]any
	return c.doJSON(ctx, token, http.MethodDelete, wabaID+"/message_templates?"+q.Encode(), nil, &out, true)
}

func (c *Client) doJSON(ctx context.Context, token, method, path string, payload any, out any, idempotent bool) error {
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}
	ctx, cancel := c.withTimeout(ctx, c.timeout)
	defer cancel()
	return c.doRaw(ctx, token, method, path, "application/json", body, out, idempotent)
}

func (c *Client) withTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

func (c *Client) doRaw(ctx context.Context, token, method, path, contentType string, body []byte, out any, idempotent bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if idempotent {
		if err := c.limiter.Wait(ctx); err != nil {
			return err
		}
	}
	attempts := 1
	if idempotent {
		attempts = 3
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.url(path), bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		req.Header.Set("User-Agent", "WaMux")
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
		} else if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if out != nil {
				decErr := json.NewDecoder(io.LimitReader(resp.Body, 10<<20)).Decode(out)
				// Empty bodies are valid for DELETE and register in some
				// Graph versions.
				if decErr != nil && !errors.Is(decErr, io.EOF) {
					lastErr = decErr
				}
			}
			resp.Body.Close()
			c.limiter.Observe(resp)
			if lastErr != nil {
				return lastErr
			}
			return nil
		} else {
			lastErr = graphErrorFromResponse(resp, method, req.URL.String())
			resp.Body.Close()
			c.limiter.Observe(resp)
		}
		if !idempotent || !isRetryable(lastErr) || attempt == attempts-1 {
			break
		}
		delay := retryDelay(attempt, lastErr)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return lastErr
}

func (c *Client) url(path string) string {
	base := strings.TrimRight(c.baseURL, "/")
	version := strings.Trim(c.version, "/")
	return base + "/" + version + "/" + strings.TrimLeft(path, "/")
}

func graphErrorFromResponse(resp *http.Response, method, url string) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var envelope struct {
		Error struct {
			Message   string `json:"message"`
			Type      string `json:"type"`
			Code      int    `json:"code"`
			Subcode   int    `json:"error_subcode"`
			UserTitle string `json:"error_user_title"`
			UserMsg   string `json:"error_user_msg"`
			TraceID   string `json:"fbtrace_id"`
			Data      struct {
				Details string `json:"details"`
			} `json:"error_data"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &envelope)
	return &GraphError{
		StatusCode: resp.StatusCode,
		Method:     method,
		URL:        url,
		Type:       envelope.Error.Type,
		Message:    envelope.Error.Message,
		Code:       envelope.Error.Code,
		Subcode:    envelope.Error.Subcode,
		UserTitle:  envelope.Error.UserTitle,
		UserMsg:    envelope.Error.UserMsg,
		Details:    envelope.Error.Data.Details,
		TraceID:    envelope.Error.TraceID,
		RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
		Raw:        raw,
	}
}

func isRetryable(err error) bool {
	var ge *GraphError
	if errors.As(err, &ge) {
		return ge.IsRetryable()
	}
	var ne net.Error
	return errors.As(err, &ne)
}

func parseRetryAfter(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if secs, err := strconv.ParseFloat(raw, 64); err == nil {
		if secs > 0 {
			return time.Duration(secs * float64(time.Second))
		}
		return 0
	}
	if t, err := http.ParseTime(raw); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

func retryDelay(attempt int, err error) time.Duration {
	var ge *GraphError
	if errors.As(err, &ge) && ge.RetryAfter > 0 {
		return ge.RetryAfter
	}
	base := 500 * time.Millisecond
	d := base * time.Duration(1<<attempt)
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	// Full jitter: a fixed retry storm is worse than a slightly late retry.
	return time.Duration(time.Now().UnixNano()%int64(d+1)) * time.Nanosecond
}

// adaptiveLimiter is a conservative floor plus a 429/usage-driven slowdown.
type adaptiveLimiter struct {
	mu         sync.Mutex
	baseRPS    float64
	currentRPS float64
	last       time.Time
	lastRaise  time.Time
}

func newAdaptiveLimiter(rps float64) *adaptiveLimiter {
	if rps <= 0 {
		rps = 8
	}
	return &adaptiveLimiter{baseRPS: rps, currentRPS: rps}
}

func (l *adaptiveLimiter) RPS() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.currentRPS
}

func (l *adaptiveLimiter) Wait(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	l.mu.Lock()
	now := time.Now()
	rps := l.currentRPS
	if rps <= 0 {
		l.last = now
		l.mu.Unlock()
		return ctx.Err()
	}
	interval := time.Duration(float64(time.Second) / rps)
	next := l.last.Add(interval)
	if next.Before(now) || l.last.IsZero() {
		l.last = now
		l.mu.Unlock()
		return nil
	}
	l.last = next
	l.mu.Unlock()
	timer := time.NewTimer(next.Sub(now))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (l *adaptiveLimiter) Observe(resp *http.Response) {
	if resp == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if resp.StatusCode == http.StatusTooManyRequests && l.baseRPS > 0 {
		l.currentRPS /= 2
		if min := l.baseRPS / 16; l.currentRPS < min {
			l.currentRPS = min
		}
		l.lastRaise = time.Now()
	}
	// X-Business-Use-Case-Usage carries percentage objects; a value near 90
	// is treated as a soft 429.
	for _, header := range []string{"X-Business-Use-Case-Usage", "X-RateLimit-Remaining"} {
		if v := strings.TrimSpace(resp.Header.Get(header)); v != "" {
			if pct, ok := usagePercent(v); ok && pct >= 90 {
				l.currentRPS /= 2
				if min := l.baseRPS / 16; l.currentRPS < min {
					l.currentRPS = min
				}
				l.lastRaise = time.Now()
			}
		}
	}
	if time.Since(l.lastRaise) > 10*time.Second && l.currentRPS < l.baseRPS {
		l.currentRPS *= 1.25
		if l.currentRPS > l.baseRPS {
			l.currentRPS = l.baseRPS
		}
		l.lastRaise = time.Now()
	}
}

func usagePercent(raw string) (float64, bool) {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		if n, err := strconv.ParseFloat(raw, 64); err == nil {
			return n, true
		}
		return 0, false
	}
	var max float64
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case map[string]any:
			for _, v := range t {
				walk(v)
			}
		case []any:
			for _, v := range t {
				walk(v)
			}
		case float64:
			if t > max {
				max = t
			}
		}
	}
	walk(v)
	return max, max > 0
}
