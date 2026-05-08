package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

// VerifySignature compares the X-Hub-Signature-256 header against an HMAC
// of the request body using the shared secret. Returns nil if valid.
func VerifySignature(signatureHeader, secret string, body []byte) error {
	if signatureHeader == "" {
		return errors.New("missing X-Hub-Signature-256")
	}
	if secret == "" {
		return errors.New("webhook secret not configured")
	}
	const prefix = "sha256="
	if !strings.HasPrefix(signatureHeader, prefix) {
		return errors.New("signature must start with sha256=")
	}
	gotSig, err := hex.DecodeString(signatureHeader[len(prefix):])
	if err != nil {
		return errors.New("malformed signature")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	wantSig := mac.Sum(nil)
	if !hmac.Equal(gotSig, wantSig) {
		return errors.New("signature mismatch")
	}
	return nil
}

// PushEvent captures the small slice of GitHub's push payload we care about.
type PushEvent struct {
	Ref     string `json:"ref"`     // refs/heads/main
	After   string `json:"after"`   // commit SHA
	Repo    struct {
		FullName string `json:"full_name"` // owner/repo
		HTMLURL  string `json:"html_url"`
	} `json:"repository"`
	Pusher struct {
		Name string `json:"name"`
	} `json:"pusher"`
}

// ParsePushEvent unmarshals a GitHub push event body.
func ParsePushEvent(body []byte) (*PushEvent, error) {
	var p PushEvent
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// BranchFromRef returns "main" from "refs/heads/main", or the input if it
// doesn't match the heads/* form.
func BranchFromRef(ref string) string {
	const p = "refs/heads/"
	if strings.HasPrefix(ref, p) {
		return ref[len(p):]
	}
	return ref
}
