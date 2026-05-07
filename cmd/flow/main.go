package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lyzrai/flow/pkg/api"
	"github.com/lyzrai/flow/pkg/engine"
	"github.com/lyzrai/flow/pkg/executors"
	"github.com/lyzrai/flow/pkg/orchestrator"
	"github.com/lyzrai/flow/web"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "run":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: flow run <workflow.json>")
			os.Exit(2)
		}
		os.Exit(runWorkflow(os.Args[2]))
	case "serve":
		os.Exit(serve())
	case "version":
		fmt.Println("flow v0.0.1-dev")
	default:
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `flow — durable n8n-compatible workflow engine

usage:
  flow run <workflow.json>   execute a workflow file once and print outputs
  flow serve                 start the HTTP server (UI + API) on :8080
  flow version               print version

env vars (for `+"`flow serve`"+`):
  FLOW_ADDR                  HTTP listen address (default :8080)
  RESTATE_INGRESS_URL        Restate ingress URL (default http://localhost:8081)
  RESTATE_ADMIN_URL          Restate admin URL (default http://localhost:9070)
  RESTATE_SERVICE_ADDR       Restate service-endpoint listen addr (default :9080)
  RESTATE_DEPLOYMENT_URI     How Restate reaches this service (default http://localhost:9080)`)
}

func runWorkflow(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		slog.Error("failed to read workflow file", slog.String("path", path), slog.Any("error", err))
		return 1
	}

	wf, err := engine.ParseWorkflow(data)
	if err != nil {
		slog.Error("failed to parse workflow", slog.Any("error", err))
		return 1
	}

	executors.RegisterAll()

	ctx := context.Background()
	result, err := engine.RunWorkflow(ctx, wf, nil, executors.BuildLookup())
	if err != nil {
		slog.Error("workflow run failed", slog.Any("error", err))
		return 1
	}

	out, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		slog.Error("failed to marshal result", slog.Any("error", err))
		return 1
	}
	fmt.Println(string(out))
	return 0
}

func serve() int {
	executors.RegisterAll()
	lookup := executors.BuildLookup()

	addr := envOr("FLOW_ADDR", ":8080")
	ingressURL := envOr("RESTATE_INGRESS_URL", "http://localhost:8081")
	adminURL := envOr("RESTATE_ADMIN_URL", "http://localhost:9070")
	serviceAddr := envOr("RESTATE_SERVICE_ADDR", ":9080")
	deployURI := envOr("RESTATE_DEPLOYMENT_URI", "http://localhost"+serviceAddr)

	slog.Info("checking restate",
		slog.String("ingress", ingressURL),
		slog.String("admin", adminURL),
	)
	if err := orchestrator.HealthCheckRestate(ingressURL); err != nil {
		slog.Error("restate not reachable, exiting",
			slog.String("hint", "run `docker compose up restate` (or set RESTATE_INGRESS_URL=)"),
			slog.Any("error", err),
		)
		return 1
	}

	orch := orchestrator.NewRestateOrchestrator(ingressURL)

	// Start the Restate service endpoint that Restate calls back into.
	go startRestateService(serviceAddr, lookup)

	// Auto-register with Restate admin so callbacks land on us.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := orchestrator.RegisterDeploymentWithRetry(ctx, adminURL, deployURI, 30, 2*time.Second); err != nil {
			slog.Error("restate deployment registration failed",
				slog.String("admin", adminURL),
				slog.String("deploy_uri", deployURI),
				slog.Any("error", err),
			)
			return
		}
		slog.Info("registered with restate", slog.String("deploy_uri", deployURI))
	}()

	// HTTP server (UI + API).
	srv := &http.Server{
		Addr: addr,
		Handler: api.NewServer(api.ServerDeps{
			Assets:            web.Dist(),
			Orchestrator:      orch,
			RestateIngressURL: orch.IngressURL(),
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("flow listening", slog.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", slog.Any("error", err))
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	slog.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("forced shutdown", slog.Any("error", err))
		return 1
	}
	return 0
}

func startRestateService(addr string, lookup engine.ExecutorLookup) {
	rs := orchestrator.NewRestateServer(lookup, nil, nil)
	slog.Info("starting restate service endpoint", slog.String("addr", addr))
	if err := rs.Start(context.Background(), addr); err != nil {
		slog.Error("restate service failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
