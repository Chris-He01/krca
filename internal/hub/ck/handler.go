package ck

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"
)

// Handler exposes the gateway client over an HTTP endpoint. Mount it with
// (*Handler).RegisterRoutes(mux) — typically at /v1/tools/ck/query.
type Handler struct {
	client  *Client
	enabled bool
}

// NewHandler builds a Handler. Pass enabled=false to mount routes that
// politely return 503 — useful when the hub starts without a CK token in
// non-prod environments.
func NewHandler(client *Client, enabled bool) *Handler {
	return &Handler{client: client, enabled: enabled}
}

// RegisterRoutes wires the handler onto the given mux. Routes:
//
//	POST /v1/tools/ck/query       structured JSON
//	POST /v1/tools/ck/query/raw   pass-through of the gateway body
//	GET  /v1/tools/ck/status      health/config probe
//	POST /v1/tools/ck/node-health typed query for service_portrait_ssd.node_label
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/tools/ck/query", h.handleQuery)
	mux.HandleFunc("/v1/tools/ck/query/raw", h.handleQueryRaw)
	mux.HandleFunc("/v1/tools/ck/status", h.handleStatus)
	mux.HandleFunc("/v1/tools/ck/node-health", h.handleNodeHealth)
}

type queryRequest struct {
	SQL            string         `json:"sql"`
	Params         map[string]any `json:"params,omitempty"`
	Limit          int            `json:"limit,omitempty"`
	TimeoutSeconds int            `json:"timeout_seconds,omitempty"`
	QueryID        string         `json:"query_id,omitempty"`
}

type queryResponse struct {
	OK     bool    `json:"ok"`
	Error  string  `json:"error,omitempty"`
	Result *Result `json:"result,omitempty"`
}

func (h *Handler) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONErr(w, http.StatusMethodNotAllowed, "method must be POST")
		return
	}
	if !h.enabled || h.client == nil {
		writeJSONErr(w, http.StatusServiceUnavailable, "ck endpoint disabled — set KNSIGHT_CK_ENABLED=true and provide a token")
		return
	}
	var req queryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if strings.TrimSpace(req.SQL) == "" {
		writeJSONErr(w, http.StatusBadRequest, "sql is required")
		return
	}
	opts := QueryOptions{
		Params:  req.Params,
		Limit:   req.Limit,
		QueryID: req.QueryID,
	}
	if req.TimeoutSeconds > 0 {
		opts.Timeout = time.Duration(req.TimeoutSeconds) * time.Second
	}

	result, err := h.client.QueryRows(r.Context(), req.SQL, opts)
	if err != nil {
		qid := ""
		if result != nil {
			qid = result.QueryID
		}
		log.Printf("[ck] query failed qid=%s err=%v", qid, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(queryResponse{
			OK:     false,
			Error:  err.Error(),
			Result: result,
		})
		return
	}
	log.Printf("[ck] query ok qid=%s rows=%d elapsed_ms=%d", result.QueryID, result.RowCount, result.ElapsedMs)

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(queryResponse{OK: true, Result: result})
}

func (h *Handler) handleQueryRaw(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONErr(w, http.StatusMethodNotAllowed, "method must be POST")
		return
	}
	if !h.enabled || h.client == nil {
		writeJSONErr(w, http.StatusServiceUnavailable, "ck endpoint disabled")
		return
	}
	var req queryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if strings.TrimSpace(req.SQL) == "" {
		writeJSONErr(w, http.StatusBadRequest, "sql is required")
		return
	}
	opts := QueryOptions{
		Params:  req.Params,
		Limit:   req.Limit,
		QueryID: req.QueryID,
	}
	if req.TimeoutSeconds > 0 {
		opts.Timeout = time.Duration(req.TimeoutSeconds) * time.Second
	}
	body, qid, err := h.client.QueryRaw(r.Context(), req.SQL, opts)
	w.Header().Set("Ks-Query-Id", qid)
	if err != nil {
		log.Printf("[ck] raw query failed qid=%s err=%v", qid, err)
		writeJSONErr(w, http.StatusBadGateway, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONErr(w, http.StatusMethodNotAllowed, "method must be GET")
		return
	}
	status := map[string]any{
		"enabled": h.enabled && h.client != nil,
	}
	if h.client != nil {
		status["gateway_url"] = h.client.cfg.GatewayURL
		status["user"] = h.client.cfg.User
		status["principal"] = h.client.cfg.Principal
		status["default_limit"] = h.client.cfg.DefaultLimit
		status["timeout_seconds"] = int(h.client.cfg.DefaultTimeout / time.Second)
	}
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(status)
}

func writeJSONErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(queryResponse{OK: false, Error: msg})
}

// ErrDisabled is returned when callers try to invoke a CK function without
// the endpoint enabled. Exported for tests/callers that want to detect it.
var ErrDisabled = errors.New("ck: endpoint disabled")
