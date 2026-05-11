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

// PullRequest is the slice of GitHub's PR shape we return upward.
type PullRequest struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
	State   string `json:"state"`
	Merged  bool   `json:"merged"`
	// Head/base sha + ref are useful for audit; pulled from nested fields.
	HeadSHA  string `json:"-"`
	HeadRef  string `json:"-"`
	BaseSHA  string `json:"-"`
	BaseRef  string `json:"-"`
	MergeSHA string `json:"-"`
}

type prRaw struct {
	Number   int    `json:"number"`
	HTMLURL  string `json:"html_url"`
	State    string `json:"state"`
	Merged   bool   `json:"merged"`
	MergedAt string `json:"merged_at"`
	MergeSHA string `json:"merge_commit_sha"`
	Head     struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"base"`
}

func (r prRaw) into() PullRequest {
	return PullRequest{
		Number: r.Number, HTMLURL: r.HTMLURL,
		State: r.State, Merged: r.Merged,
		HeadSHA: r.Head.SHA, HeadRef: r.Head.Ref,
		BaseSHA: r.Base.SHA, BaseRef: r.Base.Ref,
		MergeSHA: r.MergeSHA,
	}
}

// OpenPullRequest opens a PR from `head` into `base`. Returns the existing
// PR if one already exists for the same head/base pair (GitHub returns 422
// in that case; we re-fetch via the list API). The PAT must have `repo`
// (write) permissions.
func (c *Client) OpenPullRequest(ctx context.Context, repo Repo, head, base, title, body string) (*PullRequest, error) {
	payload := map[string]any{
		"title": firstNonEmpty(title, fmt.Sprintf("Promote %s → %s", head, base)),
		"body":  body,
		"head":  head,
		"base":  base,
	}
	buf, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		apiBase+"/repos/"+repo.String()+"/pulls", bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	c.applyAuth(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusCreated:
		var pr prRaw
		if err := json.Unmarshal(respBody, &pr); err != nil {
			return nil, fmt.Errorf("decode PR response: %w", err)
		}
		out := pr.into()
		return &out, nil
	case http.StatusUnprocessableEntity:
		// Most common cause: a PR already exists for this head/base. Fetch
		// it so promote stays idempotent.
		if existing, err := c.findPR(ctx, repo, head, base); err == nil && existing != nil {
			return existing, nil
		}
		// Otherwise surface the original 422 (could be: no commits between
		// branches, head doesn't exist, base doesn't exist, etc.).
		return nil, fmt.Errorf("open PR: github 422: %s", truncate(string(respBody), 240))
	default:
		return nil, fmt.Errorf("open PR: github returned %d: %s",
			resp.StatusCode, truncate(string(respBody), 240))
	}
}

// findPR queries the open PRs for a head→base match. Used to recover an
// already-open PR when OpenPullRequest's 422 means "duplicate".
func (c *Client) findPR(ctx context.Context, repo Repo, head, base string) (*PullRequest, error) {
	// GitHub expects head as `<owner>:<branch>` for cross-repo search; for
	// same-repo PRs the bare branch is enough.
	q := url.Values{}
	q.Set("state", "open")
	q.Set("head", repo.Owner+":"+head)
	q.Set("base", base)
	url := apiBase + "/repos/" + repo.String() + "/pulls?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.applyAuth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list pulls: github %d: %s", resp.StatusCode, truncate(string(body), 240))
	}
	var arr []prRaw
	if err := json.Unmarshal(body, &arr); err != nil {
		return nil, err
	}
	if len(arr) == 0 {
		return nil, nil
	}
	out := arr[0].into()
	return &out, nil
}

// MergePullRequest merges PR #number using the API's PUT /pulls/{n}/merge.
// `commitMessage` is optional. Method may be "merge" | "squash" | "rebase".
// Returns the merge commit SHA on success.
func (c *Client) MergePullRequest(ctx context.Context, repo Repo, number int, commitMessage, method string) (string, error) {
	if method == "" {
		method = "merge"
	}
	payload := map[string]any{"merge_method": method}
	if commitMessage != "" {
		payload["commit_message"] = commitMessage
	}
	buf, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/repos/%s/pulls/%d/merge", apiBase, repo.String(), number)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	c.applyAuth(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("github request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("merge PR #%d: github %d: %s",
			number, resp.StatusCode, truncate(string(body), 240))
	}
	var r struct {
		SHA     string `json:"sha"`
		Merged  bool   `json:"merged"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", err
	}
	if !r.Merged {
		return "", fmt.Errorf("merge not applied: %s", r.Message)
	}
	return r.SHA, nil
}

// MergeBranches uses POST /repos/{owner}/{name}/merges to fast-forward
// `base` to include `head`. No PR involved. Returns the merge commit SHA,
// or "" with no error when the branches are already up to date (204).
func (c *Client) MergeBranches(ctx context.Context, repo Repo, base, head, commitMessage string) (string, error) {
	payload := map[string]any{
		"base":           base,
		"head":           head,
		"commit_message": commitMessage,
	}
	buf, _ := json.Marshal(payload)
	url := apiBase + "/repos/" + repo.String() + "/merges"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	c.applyAuth(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("github request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	switch resp.StatusCode {
	case http.StatusCreated:
		var r struct{ SHA string `json:"sha"` }
		if err := json.Unmarshal(body, &r); err != nil {
			return "", err
		}
		return r.SHA, nil
	case http.StatusNoContent:
		// 204 = already up to date. Caller treats this as a soft success.
		return "", nil
	case http.StatusConflict:
		return "", fmt.Errorf("merge conflict between %s and %s — resolve manually or use mode=open-pr", base, head)
	default:
		return "", fmt.Errorf("merge branches: github %d: %s",
			resp.StatusCode, truncate(string(body), 240))
	}
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
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
