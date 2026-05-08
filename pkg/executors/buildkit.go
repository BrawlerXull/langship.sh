package executors

import (
	"context"
	"errors"
	"fmt"
	"strings"

	cliconfig "github.com/docker/cli/cli/config/configfile"
	cliconfigtypes "github.com/docker/cli/cli/config/types"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/frontend/dockerui"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth/authprovider"
	"github.com/moby/buildkit/util/progress/progressui"
	"golang.org/x/sync/errgroup"

	"github.com/lyzrai/flow/pkg/engine"
)

// dockerBuildOpts is the input to runDockerBuild.
type dockerBuildOpts struct {
	BuildKitAddr string            // tcp://buildkitd:1234 or unix:// path
	ContextDir   string            // local path to the cloned repo (build context)
	Dockerfile   string            // path to Dockerfile relative to ContextDir (default "Dockerfile")
	ImageRef     string            // full image ref to push, e.g. ghcr.io/org/agent:abc123
	Platform     string            // e.g. linux/amd64 (empty = buildkit default)
	BuildArgs    map[string]string // build-args
	Insecure     bool              // allow http registry / skip TLS verify on push
	// RegistryAuth maps registry hostname -> {username, password}. Empty
	// for anonymous (e.g. localhost:5000). For ghcr.io use the agent owner
	// + PAT.
	RegistryAuth map[string]registryCreds
}

type registryCreds struct {
	Username string
	Password string
}

// runDockerBuild solves a Dockerfile via BuildKit and pushes the image.
// Returns the tail of the BuildKit progress log on success or failure.
func runDockerBuild(ctx context.Context, opts dockerBuildOpts) (string, error) {
	if opts.BuildKitAddr == "" {
		return "", errors.New("BUILDKIT_HOST is not configured")
	}
	if opts.ImageRef == "" {
		return "", errors.New("imageRef is required")
	}
	if opts.ContextDir == "" {
		return "", errors.New("contextDir is required")
	}
	if opts.Dockerfile == "" {
		opts.Dockerfile = "Dockerfile"
	}

	bk, err := client.New(ctx, opts.BuildKitAddr)
	if err != nil {
		return "", fmt.Errorf("buildkit dial %s: %w", opts.BuildKitAddr, err)
	}
	defer bk.Close()

	dfDir, dfName := splitDockerfile(opts.Dockerfile)
	localDirs := map[string]string{
		dockerui.DefaultLocalNameContext:    opts.ContextDir,
		dockerui.DefaultLocalNameDockerfile: joinIfRel(opts.ContextDir, dfDir),
	}

	frontendAttrs := map[string]string{
		"filename": dfName,
	}
	for k, v := range opts.BuildArgs {
		frontendAttrs["build-arg:"+k] = v
	}
	if opts.Platform != "" {
		frontendAttrs["platform"] = opts.Platform
	}

	exporter := client.ExportEntry{
		Type: client.ExporterImage,
		Attrs: map[string]string{
			"name":              opts.ImageRef,
			"push":              "true",
			"registry.insecure": boolStr(opts.Insecure),
		},
	}

	// session.Attachable for registry auth
	authAttachable := newAuthAttachable(opts.RegistryAuth)

	// Capture BuildKit's progress UI line-by-line. Each line is forwarded
	// to the per-node logger (live SSE) AND appended to a tail buffer for
	// the final summary embedded in __build.log_tail.
	logger := engine.NodeLoggerFromContext(ctx)
	var statusBuf strings.Builder
	streamer := &streamingLineWriter{logger: logger, tail: &statusBuf}

	statusCh := make(chan *client.SolveStatus, 16)

	eg, gctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		_, err := bk.Solve(gctx, nil, client.SolveOpt{
			Frontend:      "dockerfile.v0",
			FrontendAttrs: frontendAttrs,
			LocalDirs:     localDirs,
			Exports:       []client.ExportEntry{exporter},
			Session:       []session.Attachable{authAttachable},
		}, statusCh)
		return err
	})
	eg.Go(func() error {
		display, derr := progressui.NewDisplay(streamer, progressui.PlainMode)
		if derr != nil {
			for range statusCh {
			}
			return derr
		}
		_, uerr := display.UpdateFrom(gctx, statusCh)
		return uerr
	})

	err = eg.Wait()
	streamer.flush() // emit any trailing partial line
	tail := lastLines(statusBuf.String(), 80)
	return tail, err
}

// newAuthAttachable returns a buildkit auth attachable populated with the
// given registry creds. Empty creds map → anonymous (still works for
// localhost:5000-style local registries).
func newAuthAttachable(creds map[string]registryCreds) session.Attachable {
	cfg := &cliconfig.ConfigFile{
		AuthConfigs: map[string]cliconfigtypes.AuthConfig{},
	}
	for host, c := range creds {
		cfg.AuthConfigs[host] = cliconfigtypes.AuthConfig{
			ServerAddress: host,
			Username:      c.Username,
			Password:      c.Password,
		}
	}
	return authprovider.NewDockerAuthProvider(cfg, nil)
}

// streamingLineWriter is an io.Writer that splits BuildKit's plain-mode
// progress output on newlines and forwards each complete line to a
// NodeLogger so the UI can tail them live. Bytes also accumulate into
// `tail` for the final log_tail summary embedded in __build.
type streamingLineWriter struct {
	logger engine.NodeLogger
	tail   *strings.Builder
	buf    []byte
}

func (w *streamingLineWriter) Write(p []byte) (int, error) {
	if w.tail != nil {
		w.tail.Write(p)
	}
	w.buf = append(w.buf, p...)
	for {
		i := indexByte(w.buf, '\n')
		if i < 0 {
			return len(p), nil
		}
		line := strings.TrimRight(string(w.buf[:i]), "\r")
		if w.logger != nil && line != "" {
			w.logger.Log(line)
		}
		w.buf = w.buf[i+1:]
	}
}

// flush emits the trailing partial line if any.
func (w *streamingLineWriter) flush() {
	if len(w.buf) == 0 {
		return
	}
	line := strings.TrimRight(string(w.buf), "\r\n")
	if w.logger != nil && line != "" {
		w.logger.Log(line)
	}
	w.buf = nil
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

func splitDockerfile(p string) (dir, name string) {
	p = strings.TrimPrefix(strings.TrimSpace(p), "./")
	if p == "" {
		return "", "Dockerfile"
	}
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i], p[i+1:]
	}
	return "", p
}

func joinIfRel(root, sub string) string {
	if sub == "" {
		return root
	}
	if strings.HasPrefix(sub, "/") {
		return sub
	}
	return strings.TrimRight(root, "/") + "/" + sub
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
