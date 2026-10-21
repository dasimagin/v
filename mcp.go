package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

// mcpTool is one tool the model can call. Handlers run on the HTTP server's
// goroutines, and may block (a manual review waits for the user's answer);
// ctx ends when the caller goes away.
type mcpTool struct {
	Name        string
	Description string
	Schema      string // JSON schema of the arguments
	Handler     func(ctx context.Context, args json.RawMessage) (string, error)
}

// mcpServer is a minimal MCP server over streamable HTTP, listening on
// localhost for one agent run. Only tools are supported, and every reply is a
// single JSON response.
type mcpServer struct {
	srv   *http.Server
	addr  string
	token string
	tools map[string]mcpTool
	order []string
}

func startMCP(tools []mcpTool) (*mcpServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		ln.Close()
		return nil, err
	}
	s := &mcpServer{addr: ln.Addr().String(), token: hex.EncodeToString(raw[:]), tools: map[string]mcpTool{}}
	for _, t := range tools {
		s.tools[t.Name] = t
		s.order = append(s.order, t.Name)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", s.serve)
	s.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go s.srv.Serve(ln)
	return s, nil
}

func (s *mcpServer) Close() { _ = s.srv.Close() }

// Config is the JSON for claude's --mcp-config, naming the server name.
func (s *mcpServer) Config(name string) string {
	b, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{name: map[string]any{
		"type":    "http",
		"url":     "http://" + s.addr + "/mcp",
		"headers": map[string]string{"Authorization": "Bearer " + s.token},
	}}})
	return string(b)
}

type rpcRequest struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func (s *mcpServer) serve(w http.ResponseWriter, r *http.Request) {
	// Only the agent we started knows the token; the Host check stops DNS rebinding.
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.token)) != 1 || r.Host != s.addr {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req rpcRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if len(req.ID) == 0 { // a notification: nothing to answer
		w.WriteHeader(http.StatusAccepted)
		return
	}
	reply := func(result any, rpcErr *rpcError) {
		out := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		if rpcErr != nil {
			out["error"] = rpcErr
		} else {
			out["result"] = result
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		if p.ProtocolVersion == "" {
			p.ProtocolVersion = "2025-06-18"
		}
		reply(map[string]any{
			"protocolVersion": p.ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "v", "version": "1"},
		}, nil)
	case "ping":
		reply(map[string]any{}, nil)
	case "tools/list":
		var list []map[string]any
		for _, name := range s.order {
			t := s.tools[name]
			list = append(list, map[string]any{"name": t.Name, "description": t.Description, "inputSchema": json.RawMessage(t.Schema)})
		}
		reply(map[string]any{"tools": list}, nil)
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			reply(nil, &rpcError{-32602, "bad params"})
			return
		}
		t, ok := s.tools[p.Name]
		if !ok {
			reply(nil, &rpcError{-32602, "unknown tool " + p.Name})
			return
		}
		text, err := t.Handler(r.Context(), p.Arguments)
		if err != nil {
			text = err.Error()
		}
		reply(map[string]any{"content": []map[string]any{{"type": "text", "text": text}}, "isError": err != nil}, nil)
	default:
		reply(nil, &rpcError{-32601, fmt.Sprintf("method not found: %s", req.Method)})
	}
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
