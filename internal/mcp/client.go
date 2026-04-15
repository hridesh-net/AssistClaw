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
	"os/exec"
	"strings"
	"sync"

	"go.uber.org/zap"
)

// ClientConfig configures a connection to an external MCP server.
type ClientConfig struct {
	Name      string    `yaml:"name"`
	Transport Transport `yaml:"transport"` // stdio | http
	// stdio fields
	Command string   `yaml:"command"` // e.g. "npx @modelcontextprotocol/server-filesystem /tmp"
	Args    []string `yaml:"args"`
	// Dir, if set, is the working directory for the stdio child process.
	Dir string `yaml:"dir"`
	// Env is appended to the process environment (each entry KEY=value).
	Env []string `yaml:"env"`
	// HTTP fields
	URL       string `yaml:"url"`
	AuthToken string `yaml:"auth_token"`
}

// RemoteTool is a tool discovered from an external MCP server.
type RemoteTool struct {
	ServerName string
	Definition ToolDefinition
	Call       func(ctx context.Context, args map[string]any) (CallToolResult, error)
}

// Client connects to an external MCP server and fetches its tools.
type Client struct {
	cfg ClientConfig
	log *zap.Logger
	mu  sync.Mutex
	seq int
	// For stdio transport:
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	// For HTTP transport: just use http.Client
	httpClient *http.Client
}

// NewClient creates a new MCP client from config.
func NewClient(cfg ClientConfig, log *zap.Logger) *Client {
	return &Client{
		cfg:        cfg,
		log:        log,
		httpClient: &http.Client{},
	}
}

// Connect initialises the connection and performs the MCP handshake.
func (c *Client) Connect(ctx context.Context) error {
	switch c.cfg.Transport {
	case TransportHTTP:
		return c.connectHTTP(ctx)
	default:
		return c.connectStdio(ctx)
	}
}

// ─────────────────────────────────────────────
// stdio transport
// ─────────────────────────────────────────────

func (c *Client) connectStdio(ctx context.Context) error {
	parts := strings.Fields(c.cfg.Command)
	if len(parts) == 0 && len(c.cfg.Args) == 0 {
		return fmt.Errorf("mcp client %q: no command specified", c.cfg.Name)
	}

	var cmdName string
	var cmdArgs []string
	if len(parts) > 0 {
		cmdName = parts[0]
		cmdArgs = append(parts[1:], c.cfg.Args...)
	} else {
		cmdArgs = c.cfg.Args
	}

	c.cmd = exec.CommandContext(ctx, cmdName, cmdArgs...)
	if strings.TrimSpace(c.cfg.Dir) != "" {
		c.cmd.Dir = c.cfg.Dir
	}
	if len(c.cfg.Env) > 0 {
		c.cmd.Env = append(os.Environ(), c.cfg.Env...)
	}
	stdin, err := c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("mcp client %q: stdin pipe: %w", c.cfg.Name, err)
	}
	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("mcp client %q: stdout pipe: %w", c.cfg.Name, err)
	}
	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("mcp client %q: start: %w", c.cfg.Name, err)
	}

	c.stdin = stdin
	c.stdout = bufio.NewScanner(stdout)
	c.stdout.Buffer(make([]byte, 1<<20), 1<<20)

	// Initialize handshake
	return c.initialize(ctx)
}

func (c *Client) initialize(ctx context.Context) error {
	params := InitializeParams{
		ProtocolVersion: SupportedProtocolVersion,
		ClientInfo:      ClientInfo{Name: "assistclaw", Version: "1.0"},
	}
	var result InitializeResult
	if err := c.rpcCall(ctx, "initialize", params, &result); err != nil {
		return fmt.Errorf("mcp initialize: %w", err)
	}
	c.log.Info("MCP client connected",
		zap.String("server", c.cfg.Name),
		zap.String("version", result.ProtocolVersion),
	)
	// Send initialized notification
	_ = c.sendNotification("initialized", nil)
	return nil
}

// ─────────────────────────────────────────────
// HTTP transport
// ─────────────────────────────────────────────

func (c *Client) connectHTTP(ctx context.Context) error {
	if c.cfg.URL == "" {
		return fmt.Errorf("mcp client %q: no URL specified", c.cfg.Name)
	}
	return c.initialize(ctx)
}

// ─────────────────────────────────────────────
// Tool discovery
// ─────────────────────────────────────────────

// ListTools fetches the tool list from the remote MCP server.
func (c *Client) ListTools(ctx context.Context) ([]RemoteTool, error) {
	var result ListToolsResult
	if err := c.rpcCall(ctx, "tools/list", nil, &result); err != nil {
		return nil, fmt.Errorf("mcp %q tools/list: %w", c.cfg.Name, err)
	}

	var remote []RemoteTool
	for _, def := range result.Tools {
		toolDef := def // capture for closure
		serverName := c.cfg.Name
		remote = append(remote, RemoteTool{
			ServerName: serverName,
			Definition: toolDef,
			Call: func(ctx context.Context, args map[string]any) (CallToolResult, error) {
				return c.CallTool(ctx, toolDef.Name, args)
			},
		})
	}
	return remote, nil
}

// CallTool calls a named tool on the remote MCP server.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (CallToolResult, error) {
	params := CallToolParams{Name: name, Arguments: args}
	var result CallToolResult
	if err := c.rpcCall(ctx, "tools/call", params, &result); err != nil {
		return CallToolResult{}, fmt.Errorf("mcp %q tools/call %q: %w", c.cfg.Name, name, err)
	}
	return result, nil
}

// ─────────────────────────────────────────────
// RPC transport layer
// ─────────────────────────────────────────────

func (c *Client) rpcCall(ctx context.Context, method string, params any, result any) error {
	c.mu.Lock()
	c.seq++
	id := c.seq
	c.mu.Unlock()

	req := Request{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Method:  method,
	}
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		req.Params = b
	}

	var resp Response
	switch c.cfg.Transport {
	case TransportHTTP:
		if err := c.httpRPC(ctx, req, &resp); err != nil {
			return err
		}
	default:
		if err := c.stdioRPC(ctx, req, &resp); err != nil {
			return err
		}
	}

	if resp.Error != nil {
		return resp.Error
	}
	if result != nil && resp.Result != nil {
		return json.Unmarshal(resp.Result, result)
	}
	return nil
}

func (c *Client) stdioRPC(_ context.Context, req Request, resp *Response) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(c.stdin, "%s\n", data)
	if err != nil {
		return fmt.Errorf("mcp write: %w", err)
	}

	if !c.stdout.Scan() {
		return fmt.Errorf("mcp read: connection closed")
	}
	return json.Unmarshal(c.stdout.Bytes(), resp)
}

func (c *Client) httpRPC(ctx context.Context, req Request, resp *Response) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(c.cfg.URL, "/")+"/mcp",
		bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.cfg.AuthToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.cfg.AuthToken)
	}

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("mcp http: %w", err)
	}
	defer httpResp.Body.Close()

	return json.NewDecoder(httpResp.Body).Decode(resp)
}

func (c *Client) sendNotification(method string, params any) error {
	notif := Request{JSONRPC: JSONRPCVersion, Method: method}
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		notif.Params = b
	}
	data, _ := json.Marshal(notif)
	_, err := fmt.Fprintf(c.stdin, "%s\n", data)
	return err
}

// Close terminates the client connection.
func (c *Client) Close() error {
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		return c.cmd.Process.Kill()
	}
	return nil
}
