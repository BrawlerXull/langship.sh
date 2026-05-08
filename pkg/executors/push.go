package executors

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"

	"github.com/lyzrai/flow/pkg/engine"
	"github.com/lyzrai/flow/pkg/models"
)

// PushExecutor copies an OCI image from one registry to another. Source
// defaults to the local registry image produced by an upstream Build node
// (read from inputs[0][0].__build.image) but can be overridden by the
// srcImage parameter. Destination is constructed from targetRegistry +
// targetImage + tag. Optional username/password authenticate the push.
//
// Implementation uses go-containerregistry's crane.Copy — same primitive
// the `crane` CLI / ko / skopeo-Go-callers use. No docker daemon required;
// works against any OCI v2 registry. The local registry (registry:5000) is
// reached via plain HTTP because we mark it insecure when constructing the
// reference.
//
// Parameters:
//   - srcImage         string  override source image ref (optional)
//   - targetRegistry   string  e.g. "ghcr.io"
//   - targetImage      string  e.g. "org/agent"
//   - tag              string  defaults to upstream __build.commit then "latest"
//   - username         string  optional
//   - password         string  optional
//   - srcInsecure      bool    treat src registry as plain HTTP (default true; the local one is)
//   - dstInsecure      bool    treat dst registry as plain HTTP (default false)
type PushExecutor struct{}

func (e *PushExecutor) Execute(ctx context.Context, node models.NodeDef, inputs [][]models.Item, _ *engine.ExecutionContext) (map[int][]models.Item, error) {
	logger := engine.NodeLoggerFromContext(ctx)

	// Source: explicit param wins, otherwise upstream __build.image.
	srcImage := strParam(node.Parameters, "srcImage", "")
	if srcImage == "" {
		srcImage = imageFromBuildOutput(inputs)
	}
	if srcImage == "" {
		return nil, errors.New("push: no source image (set srcImage or wire a Build node upstream)")
	}

	// Destination.
	dstRegistry := strings.TrimRight(strings.TrimSpace(strParam(node.Parameters, "targetRegistry", "")), "/")
	dstImage := strings.TrimSpace(strParam(node.Parameters, "targetImage", ""))
	if dstRegistry == "" || dstImage == "" {
		return nil, errors.New("push: targetRegistry and targetImage are required")
	}
	tag := strings.TrimSpace(strParam(node.Parameters, "tag", ""))
	if tag == "" {
		tag = strFirst(commitFromBuildOutput(inputs), "latest")
	}
	dstRef := fmt.Sprintf("%s/%s:%s", dstRegistry, dstImage, tag)

	username := strParam(node.Parameters, "username", "")
	password := strParam(node.Parameters, "password", "")

	srcInsecure := boolParam(node.Parameters, "srcInsecure", true)
	dstInsecure := boolParam(node.Parameters, "dstInsecure", false)

	logger.Log(fmt.Sprintf("copying %s -> %s", srcImage, dstRef))

	// Build the crane options. We layer:
	//   - context (for cancellation)
	//   - per-side insecure flag (lets us read from local registry:5000 over HTTP)
	//   - keychain for auth (only used by the destination side; explicit
	//     username/password takes precedence via WithAuth)
	srcOpts := []crane.Option{crane.WithContext(ctx)}
	dstOpts := []crane.Option{crane.WithContext(ctx)}
	if srcInsecure {
		srcOpts = append(srcOpts, crane.Insecure)
	}
	if dstInsecure {
		dstOpts = append(dstOpts, crane.Insecure)
	}
	if username != "" || password != "" {
		dstOpts = append(dstOpts, crane.WithAuth(&authn.Basic{
			Username: username,
			Password: password,
		}))
	} else {
		// Falls back to docker config / GHCR anonymous / etc.
		dstOpts = append(dstOpts, crane.WithAuthFromKeychain(authn.DefaultKeychain))
	}

	// Validate refs early so we surface a clear error.
	if _, err := name.ParseReference(srcImage); err != nil {
		return nil, fmt.Errorf("push: invalid src %q: %w", srcImage, err)
	}
	if _, err := name.ParseReference(dstRef); err != nil {
		return nil, fmt.Errorf("push: invalid dst %q: %w", dstRef, err)
	}

	// crane.Copy doesn't accept per-side options — we emulate by pulling
	// the image with src options and pushing it with dst options.
	logger.Log("pulling source manifest…")
	img, err := crane.Pull(srcImage, srcOpts...)
	if err != nil {
		return nil, fmt.Errorf("push: pull %s: %w", srcImage, err)
	}

	logger.Log("pushing to destination…")
	started := time.Now()
	if err := crane.Push(img, dstRef, dstOpts...); err != nil {
		return nil, fmt.Errorf("push: push %s: %w", dstRef, err)
	}
	logger.Log(fmt.Sprintf("done in %s", time.Since(started).Round(100*time.Millisecond)))

	digest, derr := img.Digest()
	digestStr := ""
	if derr == nil {
		digestStr = digest.String()
	}

	// Pass-through items + a __push summary so downstream nodes can read it.
	out := make([]models.Item, 0)
	for _, in := range inputs {
		for _, item := range in {
			ci := copyItem(item)
			ci["__push"] = map[string]any{
				"src":         srcImage,
				"dst":         dstRef,
				"digest":      digestStr,
				"finished_at": time.Now().UTC(),
			}
			out = append(out, ci)
		}
	}
	if len(out) == 0 {
		out = append(out, models.Item{
			"__push": map[string]any{
				"src":         srcImage,
				"dst":         dstRef,
				"digest":      digestStr,
				"finished_at": time.Now().UTC(),
			},
		})
	}
	return map[int][]models.Item{0: out}, nil
}

// imageFromBuildOutput walks input items for an upstream Build node's
// `__build.image` field. Returns "" if not present.
func imageFromBuildOutput(inputs [][]models.Item) string {
	for _, in := range inputs {
		for _, it := range in {
			if b, ok := it["__build"].(map[string]any); ok {
				if s, ok := b["image"].(string); ok && s != "" {
					return s
				}
			}
		}
	}
	return ""
}

// commitFromBuildOutput pulls the commit SHA from an upstream Build node so
// Push can default the destination tag to that SHA.
func commitFromBuildOutput(inputs [][]models.Item) string {
	for _, in := range inputs {
		for _, it := range in {
			if b, ok := it["__build"].(map[string]any); ok {
				if s, ok := b["commit"].(string); ok && s != "" {
					return s
				}
			}
		}
	}
	return ""
}

// boolParam reads a bool with a default. Accepts native bool, "true"/"false"
// strings, and 0/1 numerics for forgiveness on the wire.
func boolParam(p map[string]any, key string, def bool) bool {
	if p == nil {
		return def
	}
	switch v := p[key].(type) {
	case bool:
		return v
	case string:
		s := strings.ToLower(strings.TrimSpace(v))
		if s == "true" || s == "1" || s == "yes" {
			return true
		}
		if s == "false" || s == "0" || s == "no" {
			return false
		}
	case float64:
		return v != 0
	}
	return def
}
