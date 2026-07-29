package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type metricsArgs struct {
	Metric string `json:"metric"`
	Window string `json:"window"`
}

func main() {
	addr := flag.String("addr", ":8091", "mock mcp listen address")
	baseURL := flag.String("base-url", "", "public base url (default: http://localhost:port)")
	flag.Parse()

	if *baseURL == "" {
		*baseURL = defaultBaseURL(*addr)
	}

	mcpServer := server.NewMCPServer("mock-mcp", mcp.LATEST_PROTOCOL_VERSION)
	mcpServer.AddTool(mcp.NewTool("server_metrics",
		mcp.WithDescription("Return mock server metrics for RCA diagnosis"),
		mcp.WithString("metric", mcp.Description("Metric name: memory|cpu|disk"), mcp.Required()),
		mcp.WithString("window", mcp.Description("Time window, e.g. 5m"), mcp.Required()),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := metricsArgs{
			Metric: "memory",
			Window: "5m",
		}
		raw, _ := json.Marshal(request.Params.Arguments)
		_ = json.Unmarshal(raw, &args)

		payload := map[string]any{
			"metric":    args.Metric,
			"window":    args.Window,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"value":     "72%",
			"swap_used": "3%",
			"trend":     "up",
			"source":    "mock-mcp",
		}
		bytes, _ := json.Marshal(payload)
		return mcp.NewToolResultText(string(bytes)), nil
	})

	sseServer := server.NewSSEServer(mcpServer, server.WithBaseURL(*baseURL))
	mux := http.NewServeMux()
	mux.Handle("/sse", sseServer.SSEHandler())
	mux.Handle("/message", sseServer.MessageHandler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	log.Printf("mock mcp listening on %s (base url %s)", *addr, *baseURL)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatalf("mock mcp server error: %v", err)
	}
}

func defaultBaseURL(addr string) string {
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return addr
	}
	if strings.HasPrefix(addr, ":") {
		return "http://localhost" + addr
	}
	return "http://" + addr
}
