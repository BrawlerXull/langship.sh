// Package github provides minimal helpers for the agent webhook flow:
// repo URL parsing, webhook install/uninstall, and PAT auth probing.
//
// Intentionally tiny — we don't need the full go-github surface. If/when
// we do, swap this for github.com/google/go-github.
package github

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const apiBase = "https://api.github.com"

// Repo identifies a GitHub repository.
type Repo struct {
	Owner string
	Name  string
}

// String returns "owner/name".
func (r Repo) String() string { return r.Owner + "/" + r.Name }

// ParseRepo extracts owner/name from common Git URL forms:
//   - https://github.com/owner/name(.git)?
//   - http://github.com/owner/name(.git)?
//   - git@github.com:owner/name(.git)?
//
// Returns an error if the host isn't github.com or the path isn't owner/name.
func ParseRepo(raw string) (Repo, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimSuffix(s, "/")
	s = strings.TrimSuffix(s, ".git")

	// SSH form
	if strings.HasPrefix(s, "git@") {
		i := strings.Index(s, ":")
		if i < 0 {
			return Repo{}, fmt.Errorf("invalid ssh url %q", raw)
		}
		host := strings.TrimPrefix(s[:i], "git@")
		if host != "github.com" {
			return Repo{}, fmt.Errorf("unsupported host %q (only github.com)", host)
		}
		return splitOwnerRepo(s[i+1:])
	}

	// HTTPS form
	u, err := url.Parse(s)
	if err != nil {
		return Repo{}, fmt.Errorf("parse url: %w", err)
	}
	if u.Host != "github.com" {
		return Repo{}, fmt.Errorf("unsupported host %q (only github.com)", u.Host)
	}
	return splitOwnerRepo(strings.Trim(u.Path, "/"))
}

func splitOwnerRepo(p string) (Repo, error) {
	parts := strings.Split(p, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return Repo{}, fmt.Errorf("expected owner/name, got %q", p)
	}
	return Repo{Owner: parts[0], Name: parts[1]}, nil
}

// GenerateSecret returns a 32-byte hex string for use as a webhook secret.
func GenerateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Client is a thin GitHub REST client using a personal access token.
type Client struct {
	pat    string
	http   *http.Client
}

// NewClient creates a new GitHub client. The PAT is used as a Bearer token.
func NewClient(pat string) *Client {
	return &Client{
		pat:  pat,
		http: &http.Client{Timeout: 15 * time.Second},
	}
}

// TestAuth probes /repos/{owner}/{name}. Returns nil on 200, an error
// describing the failure mode on anything else (including 401/404).
func (c *Client) TestAuth(ctx context.Context, repo Repo) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		apiBase+"/repos/"+repo.String(), nil)
	if err != nil {
		return err
	}
	c.applyAuth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("github request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("github returned %d: %s", resp.StatusCode, truncate(string(body), 240))
}

// HookConfig is the payload section GitHub stores for our webhook.
type HookConfig struct {
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
	Secret      string `json:"secret"`
	InsecureSSL string `json:"insecure_ssl,omitempty"`
}

// Hook is the response shape we care about from GitHub's hook endpoints.
type Hook struct {
	ID     int64    `json:"id"`
	Active bool     `json:"active"`
	Events []string `json:"events"`
	Config struct {
		URL         string `json:"url"`
		ContentType string `json:"content_type"`
	} `json:"config"`
}

// InstallWebhook creates a repo webhook firing on push events. Returns the
// hook ID (used for later uninstall).
func (c *Client) InstallWebhook(ctx context.Context, repo Repo, callbackURL, secret string) (int64, error) {
	body := map[string]any{
		"name":   "web",
		"active": true,
		"events": []string{"push"},
		"config": HookConfig{
			URL:         callbackURL,
			ContentType: "json",
			Secret:      secret,
		},
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		apiBase+"/repos/"+repo.String()+"/hooks", bytes.NewReader(buf))
	if err != nil {
		return 0, err
	}
	c.applyAuth(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("github request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return 0, fmt.Errorf("install webhook: github returned %d: %s",
			resp.StatusCode, truncate(string(respBody), 240))
	}
	var hook Hook
	if err := json.Unmarshal(respBody, &hook); err != nil {
		return 0, fmt.Errorf("decode webhook response: %w", err)
	}
	return hook.ID, nil
}

// UninstallWebhook removes a previously-created webhook by ID. A 404 is
// treated as success (idempotent removal).
func (c *Client) UninstallWebhook(ctx context.Context, repo Repo, hookID int64) error {
	if hookID == 0 {
		return errors.New("hookID is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("%s/repos/%s/hooks/%d", apiBase, repo.String(), hookID), nil)
	if err != nil {
		return err
	}
	c.applyAuth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("github request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("uninstall webhook: github returned %d: %s",
		resp.StatusCode, truncate(string(body), 240))
}

func (c *Client) applyAuth(req *http.Request) {
	if c.pat != "" {
		req.Header.Set("Authorization", "Bearer "+c.pat)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "langship-flow")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
