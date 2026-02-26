// Package mcp implements the Model Context Protocol (MCP) for AssistClaw.
// It provides:
//   - A skill-graph-optimized MCP server (stdio + HTTP-SSE)
//   - An MCP client that consumes external MCP servers
//   - A skill adapter that bridges the skill graph ↔ MCP protocol
package mcp

import (
	"encoding/json"
	"fmt"
)

// ─────────────────────────────────────────────
// JSON-RPC 2.0 base types
// ─────────────────────────────────────────────

const JSONRPCVersion = "2.0"

// Request is a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"` // string | int | nil
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError represents a JSON-RPC error.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *RPCError) Error() string { return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message) }

// Standard JSON-RPC error codes.
const (
	ErrParseError     = -32700
	ErrInvalidRequest = -32600
	ErrMethodNotFound = -32601
	ErrInvalidParams  = -32602
	ErrInternal       = -32603
)

func errResponse(id any, code int, msg string) Response {
	return Response{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Error:   &RPCError{Code: code, Message: msg},
	}
}

func okResponse(id any, result any) (Response, error) {
	b, err := json.Marshal(result)
	if err != nil {
		return Response{}, err
	}
	return Response{JSONRPC: JSONRPCVersion, ID: id, Result: b}, nil
}

// ─────────────────────────────────────────────
// MCP protocol types
// ─────────────────────────────────────────────

// ServerInfo is returned in the initialize response.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ClientInfo is sent in the initialize request.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Capabilities describes server capabilities.
type Capabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

// ToolsCapability signals that this server supports tools/list and tools/call.
type ToolsCapability struct{}

// InitializeParams is sent by the client on first connect.
type InitializeParams struct {
	ProtocolVersion string     `json:"protocolVersion"`
	ClientInfo      ClientInfo `json:"clientInfo"`
	Capabilities    any        `json:"capabilities,omitempty"`
}

// InitializeResult is sent back to the client.
type InitializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	ServerInfo      ServerInfo   `json:"serverInfo"`
	Capabilities    Capabilities `json:"capabilities"`
	Instructions    string       `json:"instructions,omitempty"`
}

// ─────────────────────────────────────────────
// Tools
// ─────────────────────────────────────────────

// ToolDefinition describes a single tool exposed via MCP.
type ToolDefinition struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

// InputSchema is a JSON Schema object describing tool parameters.
type InputSchema struct {
	Type       string                    `json:"type"`
	Properties map[string]SchemaProperty `json:"properties,omitempty"`
	Required   []string                  `json:"required,omitempty"`
}

// SchemaProperty is a single JSON Schema property.
type SchemaProperty struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Enum        []any  `json:"enum,omitempty"`
}

// ListToolsResult is returned from tools/list.
type ListToolsResult struct {
	Tools []ToolDefinition `json:"tools"`
}

// CallToolParams is sent by the client for tools/call.
type CallToolParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// CallToolResult is returned from tools/call.
type CallToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ContentBlock is a piece of response content (text or image).
type ContentBlock struct {
	Type string `json:"type"` // "text" | "image"
	Text string `json:"text,omitempty"`
	// Image fields omitted for now
}

func textResult(text string) CallToolResult {
	return CallToolResult{Content: []ContentBlock{{Type: "text", Text: text}}}
}

func errorResult(msg string) CallToolResult {
	return CallToolResult{IsError: true, Content: []ContentBlock{{Type: "text", Text: msg}}}
}

// ─────────────────────────────────────────────
// Transport
// ─────────────────────────────────────────────

// Transport identifies the MCP transport mechanism.
type Transport string

const (
	TransportStdio Transport = "stdio"
	TransportHTTP  Transport = "http"
)

// SupportedProtocolVersion is the MCP protocol version AssistClaw implements.
const SupportedProtocolVersion = "2024-11-05"
