// Package workertrigger notifies a serverless worker (Modal) that a durable
// ingestion job is ready. The notification is an optimisation: the job is
// already committed to ingestion_jobs, the worker claims it through the normal
// lease, and a scheduled sweep retries anything a lost notification misses.
// Without WORKER_TRIGGER_SECRET the client is disabled and local/Docker
// deployments rely on the polling worker exactly as before.
package workertrigger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// SecretHeader carries the shared secret. The worker compares it in constant time.
const SecretHeader = "X-Selar-Worker-Secret"

// DefaultTimeout bounds each notification so uploads never wait on the worker.
const DefaultTimeout = 3 * time.Second

// Client posts job notifications.
type Client struct {
	url     string
	secret  string
	timeout time.Duration
	http    *http.Client
}

// FromEnv reads WORKER_TRIGGER_URL (or WORKER_URL + /jobs/trigger),
// WORKER_TRIGGER_SECRET and WORKER_TRIGGER_TIMEOUT.
func FromEnv(getenv func(string) string) *Client {
	secret := strings.TrimSpace(getenv("WORKER_TRIGGER_SECRET"))
	target := strings.TrimSpace(getenv("WORKER_TRIGGER_URL"))
	if target == "" {
		if base := strings.TrimRight(strings.TrimSpace(getenv("WORKER_URL")), "/"); base != "" {
			target = base + "/jobs/trigger"
		}
	}
	timeout := DefaultTimeout
	if raw := strings.TrimSpace(getenv("WORKER_TRIGGER_TIMEOUT")); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 && parsed <= 30*time.Second {
			timeout = parsed
		}
	}
	return &Client{url: target, secret: secret, timeout: timeout, http: &http.Client{Timeout: timeout}}
}

// Enabled reports whether notifications will be sent.
func (c *Client) Enabled() bool { return c != nil && c.url != "" && c.secret != "" }

// Authorize adds the shared secret to other worker requests (for example
// /chat) when one is configured.
func (c *Client) Authorize(request *http.Request) {
	if c != nil && c.secret != "" {
		request.Header.Set(SecretHeader, c.secret)
	}
}

// Notify sends one bounded notification. Errors never include the secret.
func (c *Client) Notify(ctx context.Context, jobID string) error {
	if !c.Enabled() {
		return nil
	}
	if _, err := uuid.Parse(jobID); err != nil {
		return fmt.Errorf("invalid job id")
	}
	body, _ := json.Marshal(map[string]string{"job_id": jobID})
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("invalid worker trigger URL")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(SecretHeader, c.secret)
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("worker trigger request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("worker trigger rejected with HTTP %d", response.StatusCode)
	}
	return nil
}

// NotifyAsync sends the notification in the background, detached from the
// request context, and only logs failures (the scheduled sweep retries).
func (c *Client) NotifyAsync(ctx context.Context, jobID string) {
	if !c.Enabled() {
		return
	}
	detached := context.WithoutCancel(ctx)
	go func() {
		if err := c.Notify(detached, jobID); err != nil {
			log.Printf("worker trigger for job %s not delivered (sweep will retry): %v", jobID, err)
		}
	}()
}
