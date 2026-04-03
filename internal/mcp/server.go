package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/assistclaw/assistclaw/internal/skills"
	"go.uber.org/zap"
)

// ServerConfig configures the MCP server.
type ServerConfig struct {
	Enabled   bool      `yaml:"enabled"`
	Transport Transport `yaml:"transport"`  // stdio | http
	HTTPPort  int       `yaml:"http_port"`  // default 5173
	AuthToken string    `yaml:"auth_token"` // optional bearer token for HTTP
}

// Server is an MCP server that exposes AssistClaw's skill graph to MCP clients.
// Tool specs are NOT preloaded — only the compact Map of Content is sent,
// and the client uses read_skill_node to fetch deeper nodes on demand.
type Server struct {
	cfg      ServerConfig
	skillReg skills.Registry
	log      *zap.Logger

	// handlers registered by the skill adapter
	toolHandlers map[string]ToolHandler
	mu           sync.RWMutex
}

// ToolHandler is a function that handles a tool call.
type ToolHandler func(ctx context.Context, args map[string]any) (CallToolResult, error)

// NewServer creates a new MCP server.
func NewServer(cfg ServerConfig, skillReg skills.Registry, log *zap.Logger) *Server {
	s := &Server{
		cfg:          cfg,
		skillReg:     skillReg,
		log:          log,
		toolHandlers: make(map[string]ToolHandler),
	}
	// Register the skill-graph traversal tool
	s.RegisterTool("read_skill_node", s.handleReadSkillNode)
	return s
}

// RegisterTool adds a named tool handler to the server.
func (s *Server) RegisterTool(name string, h ToolHandler) {
	s.mu.Lock()
	s.toolHandlers[name] = h
	s.mu.Unlock()
}

// Serve starts the MCP server on the configured transport.
// Blocks until ctx is cancelled.
func (s *Server) Serve(ctx context.Context) error {
	switch s.cfg.Transport {
	case TransportHTTP:
		return s.serveHTTP(ctx)
	default:
		return s.serveStdio(ctx)
	}
}

// ─────────────────────────────────────────────
// stdio transport
// ─────────────────────────────────────────────

func (s *Server) serveStdio(ctx context.Context) error {
	s.log.Info("MCP server listening on stdio")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1<<20), 1<<20) // 1 MB buffer

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		resp := s.handle(ctx, line)
		data, err := json.Marshal(resp)
		if err != nil {
			s.log.Error("mcp: marshal error", zap.Error(err))
			continue
		}
		fmt.Fprintf(os.Stdout, "%s\n", data)
	}
	return scanner.Err()
}

// ─────────────────────────────────────────────
// HTTP-SSE transport
// ─────────────────────────────────────────────

func (s *Server) serveHTTP(ctx context.Context) error {
	port := s.cfg.HTTPPort
	if port == 0 {
		port = 5173
	}
	addr := fmt.Sprintf(":%d", port)

	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", s.httpHandler)
	mux.HandleFunc("/mcp/tools/list", s.httpListTools)
	mux.HandleFunc("/mcp/tools/call", s.httpCallTool)

	srv := &http.Server{Addr: addr, Handler: mux}
	s.log.Info("MCP server listening on HTTP", zap.String("addr", addr))

	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	return srv.ListenAndServe()
}

func (s *Server) httpHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if !s.checkAuth(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	resp := s.handle(r.Context(), body)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) httpListTools(w http.ResponseWriter, r *http.Request) {
	if !s.checkAuth(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.buildToolList())
}

func (s *Server) httpCallTool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !s.checkAuth(r) {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	var params CallToolParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	result, err := s.callTool(r.Context(), params)
	if err != nil {
		result = errorResult(err.Error())
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) checkAuth(r *http.Request) bool {
	if s.cfg.AuthToken == "" {
		return true
	}
	auth := r.Header.Get("Authorization")
	return strings.TrimPrefix(auth, "Bearer ") == s.cfg.AuthToken
}

// ─────────────────────────────────────────────
// Request dispatcher
// ─────────────────────────────────────────────

func (s *Server) handle(ctx context.Context, raw []byte) Response {
	var req Request
	if err := json.Unmarshal(raw, &req); err != nil {
		return errResponse(nil, ErrParseError, "parse error: "+err.Error())
	}

	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "initialized":
		return Response{} // no response required for notification
	case "tools/list":
		return s.handleListTools(req)
	case "tools/call":
		return s.handleCallTool(ctx, req)
	case "ping":
		resp, _ := okResponse(req.ID, map[string]any{})
		return resp
	default:
		return errResponse(req.ID, ErrMethodNotFound, "method not found: "+req.Method)
	}
}

func (s *Server) handleInitialize(req Request) Response {
	result := InitializeResult{
		ProtocolVersion: SupportedProtocolVersion,
		ServerInfo:      ServerInfo{Name: "assistclaw", Version: "1.0"},
		Capabilities:    Capabilities{Tools: &ToolsCapability{}},
		Instructions: "AssistClaw uses a skill graph for efficient token usage. " +
			"Use read_skill_node to navigate skill details before calling tools. " +
			"This reduces token usage by 90%+ compared to loading all tool specs upfront.",
	}
	resp, _ := okResponse(req.ID, result)
	return resp
}

func (s *Server) handleListTools(req Request) Response {
	resp, _ := okResponse(req.ID, s.buildToolList())
	return resp
}

// buildToolList returns the COMPACT skill index + read_skill_node.
// This is the token-efficient representation — no full tool specs.
func (s *Server) buildToolList() ListToolsResult {
	var tools []ToolDefinition

	// 1. read_skill_node — the graph traversal meta-tool
	tools = append(tools, ToolDefinition{
		Name:        "read_skill_node",
		Description: "Read a node from the skill graph to get tool details, usage instructions, or sub-nodes. Start at the skill root to discover available tools before calling them.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]SchemaProperty{
				"skill": {
					Type:        "string",
					Description: "Skill name (e.g. 'nano-pdf', 'mcp:filesystem')",
				},
				"node": {
					Type:        "string",
					Description: "Node name within the skill (e.g. 'SKILL', 'tools', 'edit'). Use 'SKILL' for the root.",
				},
			},
			Required: []string{"skill"},
		},
	})

	// 2. One compact entry per skill — name + one-line description ONLY (no tool specs)
	for _, sk := range s.skillReg.List() {
		emoji := sk.Metadata.AssistClaw.Emoji
		desc := sk.Description
		if emoji != "" {
			desc = emoji + "  " + desc
		}
		desc += "\n(Use read_skill_node to see available tools)"

		tools = append(tools, ToolDefinition{
			Name:        "skill:" + sk.Name,
			Description: desc,
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]SchemaProperty{
					"_hint": {
						Type:        "string",
						Description: "Call read_skill_node(skill='" + sk.Name + "') to see tools before using this skill.",
					},
				},
			},
		})
	}

	// 3. Registered tool handlers (external MCP tools + built-ins)
	s.mu.RLock()
	for name := range s.toolHandlers {
		if name == "read_skill_node" {
			continue // already added
		}
		tools = append(tools, ToolDefinition{
			Name:        name,
			Description: "Tool: " + name,
			InputSchema: InputSchema{Type: "object"},
		})
	}
	s.mu.RUnlock()

	return ListToolsResult{Tools: tools}
}

func (s *Server) handleCallTool(ctx context.Context, req Request) Response {
	var params CallToolParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errResponse(req.ID, ErrInvalidParams, "invalid params")
	}
	result, err := s.callTool(ctx, params)
	if err != nil {
		result = errorResult(err.Error())
	}
	resp, _ := okResponse(req.ID, result)
	return resp
}

func (s *Server) callTool(ctx context.Context, params CallToolParams) (CallToolResult, error) {
	s.mu.RLock()
	handler, ok := s.toolHandlers[params.Name]
	s.mu.RUnlock()

	if !ok {
		return errorResult(fmt.Sprintf("unknown tool: %s — use read_skill_node to discover available tools", params.Name)), nil
	}
	return handler(ctx, params.Arguments)
}

// ─────────────────────────────────────────────
// Built-in: read_skill_node
// ─────────────────────────────────────────────

func (s *Server) handleReadSkillNode(ctx context.Context, args map[string]any) (CallToolResult, error) {
	skillName, _ := args["skill"].(string)
	nodeName, _ := args["node"].(string)
	if nodeName == "" {
		nodeName = "SKILL"
	}

	// Handle "mcp:" prefix for external MCP tools
	if strings.HasPrefix(skillName, "mcp:") {
		return s.readMCPSkillNode(skillName, nodeName), nil
	}

	sk, ok := s.skillReg.Get(skillName)
	if !ok {
		return errorResult(fmt.Sprintf("skill %q not found. Available skills: %s",
			skillName, s.listSkillNames())), nil
	}

	// Return root node (SKILL.md) — includes all node names in this skill
	if nodeName == "SKILL" || nodeName == "" {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# %s\n%s\n\n", sk.Name, sk.Description))
		sb.WriteString("## Available nodes in this skill:\n")
		for name, node := range sk.Nodes {
			if name == "SKILL" {
				continue
			}
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", name, node.Summary))
		}
		sb.WriteString("\nCall read_skill_node again with a node name to get full details.")
		return textResult(sb.String()), nil
	}

	node, ok := s.skillReg.ReadSkillNode(skillName, nodeName)
	if !ok {
		return errorResult(fmt.Sprintf("node %q not found in skill %q", nodeName, skillName)), nil
	}

	content := fmt.Sprintf("# %s / %s\n\n%s", skillName, node.Name, node.Instructions)
	return textResult(content), nil
}

func (s *Server) readMCPSkillNode(skillName, nodeName string) CallToolResult {
	// Delegate to any registered external-tool handler
	serverName := strings.TrimPrefix(skillName, "mcp:")
	toolName := serverName
	if nodeName != "SKILL" && nodeName != "" {
		toolName = serverName + "/" + nodeName
	}
	s.mu.RLock()
	handler, ok := s.toolHandlers["mcp:node:"+toolName]
	s.mu.RUnlock()
	if ok {
		if result, err := handler(context.Background(), nil); err == nil {
			return result
		}
	}
	return errorResult(fmt.Sprintf("MCP skill node %s/%s not found", skillName, nodeName))
}

func (s *Server) listSkillNames() string {
	var names []string
	for _, sk := range s.skillReg.List() {
		names = append(names, sk.Name)
	}
	return strings.Join(names, ", ")
}
