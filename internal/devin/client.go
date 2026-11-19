// Package devin is a minimal client for the Devin v3 sessions API.
package devin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	orgID   string
	apiKey  string
	http    *http.Client
}

func New(baseURL, orgID, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		orgID:   orgID,
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

type CreateSessionRequest struct {
	Prompt      string   `json:"prompt"`
	Title       string   `json:"title,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	MaxACULimit int      `json:"max_acu_limit,omitempty"`
}

type PullRequest struct {
	URL   string `json:"pr_url"`
	State string `json:"pr_state"`
}

type Session struct {
	SessionID    string        `json:"session_id"`
	URL          string        `json:"url"`
	Title        string        `json:"title"`
	Status       string        `json:"status"`
	StatusDetail string        `json:"status_detail"`
	ACUsConsumed float64       `json:"acus_consumed"`
	PullRequests []PullRequest `json:"pull_requests"`
}

// Ended reports whether the session can no longer do work on its own.
// Suspended sessions are not ended: sending a message resumes them.
func (s Session) Ended() bool {
	return s.Status == "exit" || s.Status == "error"
}

type Message struct {
	EventID   string `json:"event_id"`
	Source    string `json:"source"`
	Message   string `json:"message"`
	CreatedAt int64  `json:"created_at"`
}

type messagePage struct {
	Items       []Message `json:"items"`
	EndCursor   *string   `json:"end_cursor"`
	HasNextPage bool      `json:"has_next_page"`
}

type APIError struct {
	StatusCode int
	Title      string
	Detail     string
}

func (e *APIError) Error() string {
	msg := e.Detail
	if msg == "" {
		msg = e.Title
	}
	if msg == "" {
		msg = http.StatusText(e.StatusCode)
	}
	return fmt.Sprintf("devin api: %d: %s", e.StatusCode, msg)
}

func (c *Client) CreateSession(ctx context.Context, req CreateSessionRequest) (Session, error) {
	var s Session
	err := c.do(ctx, http.MethodPost, c.sessionsPath(), nil, req, &s)
	return s, err
}

func (c *Client) GetSession(ctx context.Context, sessionID string) (Session, error) {
	var s Session
	err := c.do(ctx, http.MethodGet, c.sessionsPath()+"/"+url.PathEscape(sessionID), nil, nil, &s)
	return s, err
}

func (c *Client) SendMessage(ctx context.Context, sessionID, message string) (Session, error) {
	var s Session
	body := map[string]string{"message": message}
	err := c.do(ctx, http.MethodPost, c.sessionsPath()+"/"+url.PathEscape(sessionID)+"/messages", nil, body, &s)
	return s, err
}

// ListMessages returns every message of a session in chronological order.
func (c *Client) ListMessages(ctx context.Context, sessionID string) ([]Message, error) {
	var all []Message
	q := url.Values{"first": {"100"}}
	for page := 0; page < 100; page++ {
		var p messagePage
		path := c.sessionsPath() + "/" + url.PathEscape(sessionID) + "/messages"
		if err := c.do(ctx, http.MethodGet, path, q, nil, &p); err != nil {
			return nil, err
		}
		all = append(all, p.Items...)
		if !p.HasNextPage || p.EndCursor == nil || *p.EndCursor == "" {
			return all, nil
		}
		q.Set("after", *p.EndCursor)
	}
	return all, nil
}

func (c *Client) sessionsPath() string {
	return "/v3/organizations/" + url.PathEscape(c.orgID) + "/sessions"
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, in, out any) error {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := &APIError{StatusCode: resp.StatusCode}
		var problem struct {
			Title  string `json:"title"`
			Detail any    `json:"detail"`
		}
		if json.Unmarshal(data, &problem) == nil {
			apiErr.Title = problem.Title
			if d, ok := problem.Detail.(string); ok {
				apiErr.Detail = d
			}
		}
		return apiErr
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}
