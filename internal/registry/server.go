package registry

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"knsight-go/internal/protocol"
)

type Server struct {
	store *Store
}

func NewServer(store *Store) *Server {
	return &Server{store: store}
}

type registerRequest struct {
	Card       protocol.AgentCard `json:"card"`
	TTLSeconds int                `json:"ttl_seconds"`
}

type heartbeatRequest struct {
	AgentID string `json:"agent_id"`
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.HandleHealth)
	mux.HandleFunc("/v1/registry/register", s.HandleRegister)
	mux.HandleFunc("/v1/registry/heartbeat", s.HandleHeartbeat)
	mux.HandleFunc("/v1/registry/agents", s.HandleList)
	mux.HandleFunc("/v1/registry/agents/", s.HandleGet)
	return mux
}

func (s *Server) StartCleanupLoop(interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				removed := s.store.CleanupExpired()
				if removed > 0 {
					log.Printf("registry cleanup removed %d expired agents", removed)
				}
			case <-stop:
				return
			}
		}
	}()
}

func (s *Server) HandleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) HandleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Card.ID == "" || req.Card.Name == "" || req.Card.Endpoint == "" {
		http.Error(w, "missing agent card fields", http.StatusBadRequest)
		return
	}
	if req.TTLSeconds == 0 {
		req.TTLSeconds = 30
	}
	rec := s.store.Register(req.Card, req.TTLSeconds)
	writeJSON(w, rec)
}

func (s *Server) HandleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req heartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	rec, ok := s.store.Heartbeat(req.AgentID)
	if !ok {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	writeJSON(w, rec)
}

func (s *Server) HandleList(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	cap := r.URL.Query().Get("cap")

	agents := s.store.List()
	filtered := make([]AgentRecord, 0, len(agents))
	for _, rec := range agents {
		if name != "" && !strings.EqualFold(rec.Card.Name, name) {
			continue
		}
		if cap != "" && !hasCapability(rec.Card, cap) {
			continue
		}
		filtered = append(filtered, rec)
	}
	writeJSON(w, filtered)
}

func (s *Server) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/v1/registry/agents/")
	if id == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	rec, ok := s.store.Get(id)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	writeJSON(w, rec)
}

func hasCapability(card protocol.AgentCard, cap string) bool {
	for _, c := range card.Capabilities {
		if strings.EqualFold(c, cap) {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}
