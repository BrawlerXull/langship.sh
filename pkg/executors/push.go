package executors

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/lyzrai/flow/pkg/engine"
	"github.com/lyzrai/flow/pkg/models"
)

// PushExecutor copies an OCI image from one registry to one or more
// destinations in parallel. Each target is an independent network op with
// its own registry/image/tag/auth — so mirroring to N clouds takes
// max(targetN), not sum(targetN).
//
// Source defaults to the upstream Build node's `__build.image` (read off
// inputs[0][0].__build.image) and can be overridden by srcImage. The local
// registry is HTTP, so srcInsecure defaults to true.
//
// Targets shape (parameters.targets is a JSON array):
//
//	[
//	  {
//	    "name": "ghcr",
//	    "registry": "ghcr.io",
//	    "image": "org/agent",
//	    "tag": "v1.2.3",          # optional; defaults to upstream commit SHA, then "latest"
//	    "username": "owner",
//	    "password": "ghp_…",
//	    "insecure": false
//	  },
//	  { "name": "mirror", "registry": "registry.local:5000", "image": "org/agent", "insecure": true }
//	]
//
// Backward compat: if `targets` is empty/missing, the legacy single-target
// fields (targetRegistry / targetImage / tag / username / password /
// dstInsecure) are read.
//
// Implementation uses go-containerregistry — same primitive `crane` CLI /
// ko / skopeo-Go-callers use. No docker daemon required.
type PushExecutor struct{}

// pushTarget is the parsed shape of one entry in parameters.targets[].
type pushTarget struct {
	Name     string
	Registry string
	Image    string
	Tag      string
	Username string
	Password string
	Insecure bool
}

// pushCopy is what we record per target for the run's output items.
type pushCopy struct {
	Name       string `json:"name,omitempty"`
	Registry   string `json:"registry"`
	ImageRef   string `json:"imageRef"`
	Digest     string `json:"digest,omitempty"`
	DurationMS int64  `json:"durationMs"`
	Error      string `json:"error,omitempty"`
}

func (e *PushExecutor) Execute(ctx context.Context, node models.NodeDef, inputs [][]models.Item, _ *engine.ExecutionContext) (map[int][]models.Item, error) {
	logger := engine.NodeLoggerFromContext(ctx)

	// --- source ---
	srcImage := strParam(node.Parameters, "srcImage", "")
	if srcImage == "" {
		srcImage = imageFromBuildOutput(inputs)
	}
	if srcImage == "" {
		return nil, errors.New("push: no source image (set srcImage or wire a Build node upstream)")
	}
	// Build pushes to `registry:5000` (the compose service name), but Push
	// runs in the flow process on the host where `registry` doesn't
	// resolve. Rewrite to `localhost:5000` so crane.Pull can reach the
	// published host port. Symmetric to the rewrite ImageScan does in the
	// other direction.
	srcImage = pushSourceHostRef(srcImage)
	if _, err := name.ParseReference(srcImage); err != nil {
		return nil, fmt.Errorf("push: invalid src %q: %w", srcImage, err)
	}
	srcInsecure := boolParam(node.Parameters, "srcInsecure", true)

	// --- targets ---
	targets := parseTargets(node.Parameters, inputs)
	if len(targets) == 0 {
		return nil, errors.New("push: no targets configured (set targets[] or targetRegistry/targetImage)")
	}

	logger.Log(fmt.Sprintf("[push] source %s, %d target(s)", srcImage, len(targets)))

	// --- pull source once ---
	srcOpts := []crane.Option{crane.WithContext(ctx)}
	if srcInsecure {
		srcOpts = append(srcOpts, crane.Insecure)
	}
	logger.Log("pulling source manifest…")
	img, err := crane.Pull(srcImage, srcOpts...)
	if err != nil {
		return nil, fmt.Errorf("push: pull %s: %w", srcImage, err)
	}

	// --- fan out pushes ---
	results := make([]pushCopy, len(targets))
	var wg sync.WaitGroup
	wg.Add(len(targets))
	for i, t := range targets {
		go func(i int, t pushTarget) {
			defer wg.Done()
			results[i] = pushOne(ctx, img, t, logger)
		}(i, t)
	}
	wg.Wait()

	// --- summarize ---
	succeeded := 0
	for _, r := range results {
		if r.Error == "" {
			succeeded++
		}
	}
	logger.Log(fmt.Sprintf("[push] done: %d/%d target(s) succeeded", succeeded, len(results)))

	// Convert to JSON-friendly maps for the output items.
	copiesAny := make([]map[string]any, 0, len(results))
	for _, r := range results {
		copiesAny = append(copiesAny, map[string]any{
			"name":       r.Name,
			"registry":   r.Registry,
			"imageRef":   r.ImageRef,
			"digest":     r.Digest,
			"durationMs": r.DurationMS,
			"error":      r.Error,
		})
	}
	summary := map[string]any{
		"src":         srcImage,
		"copies":      copiesAny,
		"finished_at": time.Now().UTC(),
	}
	// Convenience top-level fields when there's only one target — keeps
	// the old `__push.dst / digest` shape working for downstream tooling.
	if len(results) == 1 {
		summary["dst"] = results[0].ImageRef
		summary["digest"] = results[0].Digest
	}

	out := make([]models.Item, 0)
	for _, in := range inputs {
		for _, item := range in {
			ci := copyItem(item)
			ci["__push"] = summary
			out = append(out, ci)
		}
	}
	if len(out) == 0 {
		out = append(out, models.Item{"__push": summary})
	}

	if succeeded != len(results) {
		// Surface the first error so the run shows a meaningful failure
		// message; the rest are still in __push.copies.
		for _, r := range results {
			if r.Error != "" {
				return nil, fmt.Errorf("push to %s failed: %s", r.ImageRef, r.Error)
			}
		}
	}
	return map[int][]models.Item{0: out}, nil
}

// pushOne pushes a previously-pulled image to one target.
func pushOne(ctx context.Context, img v1.Image, t pushTarget, logger engine.NodeLogger) pushCopy {
	started := time.Now()
	dst := fmt.Sprintf("%s/%s:%s", strings.TrimRight(t.Registry, "/"), t.Image, t.Tag)
	tag := t.tagPrefix()

	logger.Log(fmt.Sprintf("%s copying → %s", tag, dst))

	if _, err := name.ParseReference(dst); err != nil {
		logger.Log(fmt.Sprintf("%s ✗ invalid ref: %v", tag, err))
		return pushCopy{
			Name: t.Name, Registry: t.Registry, ImageRef: dst,
			DurationMS: time.Since(started).Milliseconds(),
			Error:      fmt.Sprintf("invalid ref: %v", err),
		}
	}

	dstOpts := []crane.Option{crane.WithContext(ctx)}
	if t.Insecure {
		dstOpts = append(dstOpts, crane.Insecure)
	}
	if t.Username != "" || t.Password != "" {
		dstOpts = append(dstOpts, crane.WithAuth(&authn.Basic{
			Username: t.Username,
			Password: t.Password,
		}))
	} else {
		dstOpts = append(dstOpts, crane.WithAuthFromKeychain(authn.DefaultKeychain))
	}

	if err := crane.Push(img, dst, dstOpts...); err != nil {
		logger.Log(fmt.Sprintf("%s ✗ %v", tag, err))
		return pushCopy{
			Name: t.Name, Registry: t.Registry, ImageRef: dst,
			DurationMS: time.Since(started).Milliseconds(),
			Error:      err.Error(),
		}
	}
	digestStr := ""
	if d, derr := img.Digest(); derr == nil {
		digestStr = d.String()
	}
	dur := time.Since(started)
	logger.Log(fmt.Sprintf("%s ✓ %s (%s)", tag, dst, dur.Round(100*time.Millisecond)))
	return pushCopy{
		Name: t.Name, Registry: t.Registry, ImageRef: dst,
		Digest: digestStr, DurationMS: dur.Milliseconds(),
	}
}

// tagPrefix is the per-target log line prefix. Mirrors the Node version's
// "[push:name]" so a multi-target log is greppable.
func (t pushTarget) tagPrefix() string {
	if t.Name != "" {
		return "[push:" + t.Name + "]"
	}
	return "[push:" + t.Registry + "]"
}

// parseTargets unifies the new `targets[]` shape with the old single-target
// fields. Returns nothing if neither is set (caller treats as error).
//
// Tag default order:  target.tag → upstream __build.commit → "latest".
func parseTargets(p map[string]any, inputs [][]models.Item) []pushTarget {
	commit := commitFromBuildOutput(inputs)

	defaultTag := func(explicit string) string {
		t := strings.TrimSpace(explicit)
		if t != "" {
			return t
		}
		if commit != "" {
			return commit
		}
		return "latest"
	}

	if raw, ok := p["targets"]; ok {
		if arr, ok := raw.([]any); ok && len(arr) > 0 {
			out := make([]pushTarget, 0, len(arr))
			for i, e := range arr {
				m, ok := e.(map[string]any)
				if !ok {
					continue
				}
				reg := strings.TrimSpace(strFromAny(m["registry"]))
				img := strings.TrimSpace(strFromAny(m["image"]))
				if reg == "" || img == "" {
					continue
				}
				out = append(out, pushTarget{
					Name:     defaultName(strFromAny(m["name"]), reg, i),
					Registry: reg,
					Image:    img,
					Tag:      defaultTag(strFromAny(m["tag"])),
					Username: strFromAny(m["username"]),
					Password: strFromAny(m["password"]),
					Insecure: anyToBool(m["insecure"], false),
				})
			}
			if len(out) > 0 {
				return out
			}
		}
	}

	// Legacy single-target fields.
	reg := strings.TrimSpace(strParam(p, "targetRegistry", ""))
	img := strings.TrimSpace(strParam(p, "targetImage", ""))
	if reg == "" || img == "" {
		return nil
	}
	return []pushTarget{{
		Name:     defaultName("", reg, 0),
		Registry: reg,
		Image:    img,
		Tag:      defaultTag(strParam(p, "tag", "")),
		Username: strParam(p, "username", ""),
		Password: strParam(p, "password", ""),
		Insecure: boolParam(p, "dstInsecure", false),
	}}
}

func defaultName(explicit, registry string, idx int) string {
	s := strings.TrimSpace(explicit)
	if s != "" {
		return s
	}
	// First label of the registry hostname is usually a clear ID.
	host := registry
	if i := strings.Index(host, "."); i > 0 {
		host = host[:i]
	}
	if host == "" {
		return fmt.Sprintf("target-%d", idx+1)
	}
	return host
}

// pushSourceHostRef rewrites a compose-internal registry hostname to its
// host-published equivalent so the host-running flow process can actually
// reach the registry. Only the most common pair we set up
// (`registry:5000` → `localhost:5000`) is rewritten; everything else
// passes through unchanged.
func pushSourceHostRef(ref string) string {
	if strings.HasPrefix(ref, "registry:5000/") {
		return "localhost:5000" + ref[len("registry:5000"):]
	}
	return ref
}

func anyToBool(v any, def bool) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		s := strings.ToLower(strings.TrimSpace(x))
		if s == "true" || s == "1" || s == "yes" {
			return true
		}
		if s == "false" || s == "0" || s == "no" {
			return false
		}
	case float64:
		return x != 0
	}
	return def
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
	return anyToBool(p[key], def)
}

