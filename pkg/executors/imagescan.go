package executors

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lyzrai/flow/pkg/engine"
	"github.com/lyzrai/flow/pkg/models"
)

// ImageScanExecutor scans an OCI image (already pushed to a registry) for
// CVEs, malware, embedded secrets, and base-image vulnerabilities. Pairs
// with a Build node upstream — ImageScan reads `__build.image` from input
// items by default — but can also scan a hand-specified ref.
//
// This is distinct from the SAST node: SAST scans *source*, ImageScan
// scans the *built artifact*. They catch different classes of bugs (a
// vulnerable transitive dep that only appears in the final layer; a
// secret baked into a layer; a malicious base image).
//
// Tools (sibling docker containers):
//   - trivy  — `trivy image <ref>` — vulns + secrets + misconfig in image
//   - grype  — `grype <ref>` — Anchore's CVE scanner
//   - custom — your container, your command. We mount nothing; it's your
//              tool's job to pull whatever it needs.
//
// Parameters:
//   - tool                 trivy | grype | custom         (default: trivy)
//   - imageRef             string                          (optional; defaults to upstream __build.image)
//   - severityThreshold    LOW | MEDIUM | HIGH | CRITICAL  (default: HIGH)
//   - failOnFinding        bool                            (default: true)
//   - timeoutSeconds       number                          (default: 600)
//   - registryUsername     string  for private source registries
//   - registryPassword     string
//   - insecure             bool    accept plain-HTTP / self-signed (default: true; matches local registry)
//   - dockerNetwork        string  override compose network (default: langship-restate_default)
//   - custom only:
//       image    string  scanner image
//       command  string  shell command (image ref is exported as $IMAGE_REF)
type ImageScanExecutor struct{}

func (e *ImageScanExecutor) Execute(ctx context.Context, node models.NodeDef, inputs [][]models.Item, _ *engine.ExecutionContext) (map[int][]models.Item, error) {
	logger := engine.NodeLoggerFromContext(ctx)

	// --- resolve image ref ---
	ref := strParam(node.Parameters, "imageRef", "")
	if ref == "" {
		ref = imageFromBuildOutput(inputs)
	}
	if ref == "" {
		return nil, errors.New("imageScan: no imageRef (set explicitly or wire a Build node upstream)")
	}

	tool := strings.ToLower(strParam(node.Parameters, "tool", "trivy"))
	threshold := strings.ToUpper(strParam(node.Parameters, "severityThreshold", "HIGH"))
	failOnFinding := boolParam(node.Parameters, "failOnFinding", true)
	timeoutSec := intParam(node.Parameters, "timeoutSeconds", 600)
	if timeoutSec < 30 {
		timeoutSec = 30
	}
	if timeoutSec > 3600 {
		timeoutSec = 3600
	}
	insecure := boolParam(node.Parameters, "insecure", true)
	username := strParam(node.Parameters, "registryUsername", "")
	password := strParam(node.Parameters, "registryPassword", "")
	dockerNetwork := strParam(node.Parameters, "dockerNetwork", "langship-restate_default")

	hardCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	// --- registry-network ref rewrite ---
	// Build pushes to `registry:5000/...` (the compose service name) but the
	// flow process running on the host stamps `__build.image` with that same
	// ref. From the host, `registry` doesn't resolve — but we run the scanner
	// inside the compose network where it does. So we leave the ref intact
	// and run docker on the same network. If the ref is a localhost/127.x
	// address, swap to `registry:5000` so the scanner-in-network can reach it.
	scanRef := registryNetworkRef(ref)

	logger.Log(fmt.Sprintf("[imagescan:%s] scanning %s", tool, scanRef))

	// --- optional auth via mounted docker config ---
	var configMount []string
	if username != "" || password != "" {
		dir, cleanup, err := writeDockerConfig(scanRef, username, password)
		if err != nil {
			return nil, fmt.Errorf("imageScan: write docker config: %w", err)
		}
		defer cleanup()
		configMount = []string{"-v", dir + ":/root/.docker:ro"}
	}

	var (
		findings []sastFinding
		toolErr  error
	)
	switch tool {
	case "trivy":
		findings, toolErr = runTrivyImage(hardCtx, scanRef, threshold, insecure, configMount, dockerNetwork, logger)
	case "grype":
		findings, toolErr = runGrype(hardCtx, scanRef, insecure, configMount, dockerNetwork, logger)
	case "custom":
		findings, toolErr = runCustomImage(hardCtx, node.Parameters, scanRef, dockerNetwork, logger)
	default:
		return nil, fmt.Errorf("imageScan: unknown tool %q", tool)
	}
	if toolErr != nil {
		return nil, fmt.Errorf("imageScan (%s): %w", tool, toolErr)
	}

	counts := map[string]int{}
	for _, f := range findings {
		counts[strings.ToUpper(f.Severity)]++
	}
	logger.Log(fmt.Sprintf("[imagescan:%s] %d finding(s) %v", tool, len(findings), counts))

	out := map[string]any{
		"tool":              tool,
		"imageRef":          ref,        // original ref users care about
		"scanRef":           scanRef,    // rewritten ref the scanner used
		"severityThreshold": threshold,
		"counts":            counts,
		"finding_count":     len(findings),
		"findings":          findings,
		"finished_at":       time.Now().UTC(),
	}

	items := make([]models.Item, 0)
	for _, in := range inputs {
		for _, it := range in {
			ci := copyItem(it)
			ci["__imageScan"] = out
			items = append(items, ci)
		}
	}
	if len(items) == 0 {
		items = append(items, models.Item{"__imageScan": out})
	}

	if failOnFinding && exceedsThreshold(findings, threshold) {
		return nil, fmt.Errorf("imageScan (%s) failed: severity threshold %s exceeded (%v)",
			tool, threshold, counts)
	}
	return map[int][]models.Item{0: items}, nil
}

// --- trivy image ---------------------------------------------------------

func runTrivyImage(ctx context.Context, ref, threshold string, insecure bool, configMount []string, network string, logger engine.NodeLogger) ([]sastFinding, error) {
	args := []string{"run", "--rm", "--network", network}
	args = append(args, configMount...)
	args = append(args,
		"aquasec/trivy:latest",
		"image", "--quiet",
		"--format", "json",
		"--severity", severityChainAtOrAbove(threshold),
		"--scanners", "vuln,secret",
	)
	if insecure {
		args = append(args, "--insecure")
	}
	args = append(args, ref)

	out, err := dockerRunCapture(ctx, args, logger)
	if err != nil && len(out) == 0 {
		return nil, err
	}
	return parseTrivy(out) // same JSON shape as `trivy fs`
}

// --- grype ---------------------------------------------------------------

func runGrype(ctx context.Context, ref string, insecure bool, configMount []string, network string, logger engine.NodeLogger) ([]sastFinding, error) {
	args := []string{"run", "--rm", "--network", network}
	args = append(args, configMount...)
	if insecure {
		// grype trusts the docker-config "insecureRegistries" too, but the
		// quickest knob is its env var.
		args = append(args, "-e", "GRYPE_REGISTRY_INSECURE_USE_HTTP=true")
		args = append(args, "-e", "GRYPE_REGISTRY_INSECURE_SKIP_TLS_VERIFY=true")
	}
	args = append(args,
		"anchore/grype:latest",
		ref,
		"-o", "json",
	)
	out, err := dockerRunCapture(ctx, args, logger)
	if err != nil && len(out) == 0 {
		return nil, err
	}
	return parseGrype(out)
}

type grypeReport struct {
	Matches []struct {
		Vulnerability struct {
			ID       string `json:"id"`
			Severity string `json:"severity"`
		} `json:"vulnerability"`
		Artifact struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"artifact"`
	} `json:"matches"`
}

func parseGrype(raw []byte) ([]sastFinding, error) {
	if i := bytes.IndexByte(raw, '{'); i > 0 {
		raw = raw[i:]
	}
	var r grypeReport
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("parse grype json: %w", err)
	}
	out := make([]sastFinding, 0, len(r.Matches))
	for _, m := range r.Matches {
		out = append(out, sastFinding{
			Tool:     "grype",
			Severity: strings.ToUpper(m.Vulnerability.Severity),
			RuleID:   m.Vulnerability.ID,
			Message: fmt.Sprintf("%s in %s@%s",
				m.Vulnerability.ID, m.Artifact.Name, m.Artifact.Version),
		})
	}
	return out, nil
}

// --- custom --------------------------------------------------------------

func runCustomImage(ctx context.Context, p map[string]any, ref, network string, logger engine.NodeLogger) ([]sastFinding, error) {
	image := strings.TrimSpace(strParam(p, "image", ""))
	command := strings.TrimSpace(strParam(p, "command", ""))
	if image == "" || command == "" {
		return nil, errors.New("custom imageScan requires image and command")
	}
	args := []string{
		"run", "--rm", "--network", network,
		"-e", "IMAGE_REF=" + ref,
		image,
		"/bin/sh", "-c", command,
	}
	out, err := dockerRunCapture(ctx, args, logger)
	if err != nil {
		return nil, fmt.Errorf("custom scanner exit: %w (last: %s)", err, oneLineSummary(string(out)))
	}
	return []sastFinding{{
		Tool:     "custom",
		Severity: "UNKNOWN",
		Message:  lastLines(string(out), 20),
	}}, nil
}

// --- helpers -------------------------------------------------------------

// registryNetworkRef rewrites localhost-flavoured registry refs so the
// scanner-in-network can reach the bundled `registry:5000`. Anything else
// passes through.
func registryNetworkRef(ref string) string {
	for _, host := range []string{"localhost:5000", "127.0.0.1:5000", "host.docker.internal:5000"} {
		if strings.HasPrefix(ref, host+"/") {
			return "registry:5000" + ref[len(host):]
		}
	}
	return ref
}

// writeDockerConfig drops a docker config.json into a temp dir so the
// scanner container picks up creds when mounted at /root/.docker. The host
// of the registry is parsed off the image ref. Returns (dir, cleanup, err).
func writeDockerConfig(ref, username, password string) (string, func(), error) {
	host := registryHostFromRef(ref)
	if host == "" {
		return "", func() {}, errors.New("could not parse registry host from image ref")
	}
	dir, err := os.MkdirTemp("", "flow-imagescan-")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }

	// docker-config "auths" expects the credential as base64(user:pass).
	authB64 := base64Encode(username + ":" + password)
	cfg := fmt.Sprintf(`{"auths":{%q:{"auth":%q}}}`, host, authB64)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(cfg), 0o600); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return dir, cleanup, nil
}

// registryHostFromRef returns the registry hostname (and optional port)
// from an OCI ref. "registry:5000/owner/img:tag" → "registry:5000".
func registryHostFromRef(ref string) string {
	i := strings.Index(ref, "/")
	if i < 0 {
		return ""
	}
	head := ref[:i]
	// A bare path with no host (e.g. "library/alpine:latest") shouldn't get
	// here — Docker treats those as docker.io.
	if !strings.Contains(head, ".") && !strings.Contains(head, ":") && head != "localhost" {
		return ""
	}
	return head
}

// base64Encode wraps stdlib for symmetry with the docker-config writer.
func base64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}
