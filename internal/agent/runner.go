// Package agent implements the AssistClaw agent runner — the main loop that
// routes messages to LLMs, dispatches tool calls, manages context, and
// writes to all three memory tiers.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/assistclaw/assistclaw/internal/channels"
	"github.com/assistclaw/assistclaw/internal/memory"
	"github.com/assistclaw/assistclaw/internal/provider"
)

// ─────────────────────────────────────────────
// Tool interface
// ─────────────────────────────────────────────

// Tool is the interface that all built-in and user-generated tools must implement.
type Tool interface {
	// Definition returns the schema passed to the LLM.
	Definition() provider.ToolDef
	// Execute runs the tool with the given JSON input.
	// Returns (output string, error). Non-fatal errors should be returned
	// as output strings so the LLM can reason about them.
	Execute(ctx context.Context, input json.RawMessage) (string, error)
}

// ToolRegistry maps tool names to implementations.
type ToolRegistry struct {
	tools map[string]Tool
}

// NewToolRegistry creates an empty tool registry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: make(map[string]Tool)}
}

// Register adds a tool.
func (r *ToolRegistry) Register(t Tool) {
	r.tools[t.Definition().Name] = t
}

// Get returns a tool by name.
func (r *ToolRegistry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// Definitions returns all tool definitions for LLM requests.
func (r *ToolRegistry) Definitions() []provider.ToolDef {
	defs := make([]provider.ToolDef, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, t.Definition())
	}
	return defs
}

// ─────────────────────────────────────────────
// Runner
// ─────────────────────────────────────────────

// Config holds runner-specific settings.
type Config struct {
	MaxIterations       int
	SystemPrompt        string
	Model               string
	ActiveSkillsContext string
	EnablePlanning      bool
	EnableReflection    bool
	EmbeddingModel      string
	SessionID           string // The persistent session ID for this runner
	ChannelID           string // The message channel ID (e.g. "whatsapp", "telegram")
}

// Runner is the main agent execution loop.
type Runner struct {
	cfg      Config
	provider provider.Provider
	tools    *ToolRegistry
	memory   *memory.Manager
	working  *memory.WorkingMemory // Added working memory field
	log      *zap.Logger

	sessionID    string
	channelID    string
	workspaceDir string
}

// NewRunner creates a new agent runner.
func NewRunner(
	cfg Config,
	p provider.Provider,
	tools *ToolRegistry,
	mem *memory.Manager,
	log *zap.Logger,
	workspaceDir string,
) *Runner {
	if cfg.MaxIterations == 0 {
		cfg.MaxIterations = 64
	}
	sessionID := cfg.SessionID
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	return &Runner{
		cfg:          cfg,
		provider:     p,
		tools:        tools,
		memory:       mem,
		working:      mem.GetWorking(sessionID),
		log:          log,
		sessionID:    sessionID,
		channelID:    cfg.ChannelID,
		workspaceDir: workspaceDir,
	}
}

// plan generates a multi-step execution plan for the given query.
func (r *Runner) plan(ctx context.Context, query string) (string, error) {
	prompt := fmt.Sprintf(`
You are in the **PLANNING PHASE**. Your goal is to break down the user's request into a series of logical, executable milestones.
Request: "%s"

Please provide:
1. A concise summary of the goal.
2. A numbered list of milestones.
3. Potential risks or edge cases to watch for.

Format your response inside <planning> tags. 
Do NOT call any tools yet. Just plan.
`, query)

	req := &provider.CompletionRequest{
		Model:        r.cfg.Model,
		SystemPrompt: r.buildSystemPrompt(ctx, query),
		Messages: []provider.Message{
			provider.NewTextMessage(provider.RoleUser, prompt),
		},
		MaxTokens: 2048,
	}

	resp, err := r.provider.Complete(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Text(), nil
}

// reflect critiques the agent's work against the original plan.
func (r *Runner) reflect(ctx context.Context, query string, plan string) (string, bool, error) {
	prompt := fmt.Sprintf(`
You are in the **REFLECTION PHASE**. Your goal is to critique your own work.
Original Request: "%s"
Original Plan:
%s

Review the conversation history above. Did you successfully achieve the goal? 
Provide:
1. **Self-Critique**: What went well? what failed?
2. **Status**: Return EXACTLY "SUCCESS" if the task is fully complete, or "RETRY" if more work is needed.
3. **Lesson Learned**: If something failed or was unexpectedly complex, provide a concise lesson for your future self inside <lesson_learned> tags.

Format your response inside <reflexion> tags.
`, query, plan)

	req := &provider.CompletionRequest{
		Model:        r.cfg.Model,
		SystemPrompt: r.buildSystemPrompt(ctx, query),
		Messages:     r.convertMessages(r.working.Messages()), // Use helper
		MaxTokens:    2048,
	}
	// Append the reflection prompt as the last user message
	req.Messages = append(req.Messages, provider.NewTextMessage(provider.RoleUser, prompt))

	resp, err := r.provider.Complete(ctx, req)
	if err != nil {
		return "", false, err
	}

	text := resp.Text()
	success := strings.Contains(text, "SUCCESS")
	return text, success, nil
}

// WithSession returns a new Runner clone for a specific session ID.
func (r *Runner) WithSession(sessionID string) *Runner {
	return &Runner{
		cfg:          r.cfg,
		provider:     r.provider,
		tools:        r.tools,
		memory:       r.memory,
		working:      r.memory.GetWorking(sessionID),
		log:          r.log,
		sessionID:    sessionID,
		channelID:    r.channelID,
		workspaceDir: r.workspaceDir,
	}
}

// SessionID returns the current session ID.
func (r *Runner) SessionID() string { return r.sessionID }

// RunResult holds the outcome of a Run call.
type RunResult struct {
	SessionID  string
	Response   string
	Iterations int
	Usage      provider.TokenUsage
}

// Run processes a user message and returns the assistant's final response.
// It handles the complete tool-use loop: LLM → tool calls → tool results → LLM.
func (r *Runner) Run(ctx context.Context, userMessage string) (*RunResult, error) {
	// Append user message to working memory.
	userMsg := memory.Message{
		ID:        uuid.New().String(),
		SessionID: r.sessionID,
		Role:      memory.RoleUser,
		Content:   userMessage,
		CreatedAt: time.Now(),
	}
	r.working.Append(userMsg)
	if err := r.memory.Episodic.Save(ctx, userMsg); err != nil {
		r.log.Warn("episodic save failed", zap.Error(err))
	}

	var totalUsage provider.TokenUsage
	iterations := 0

	// V3: Planning Phase
	plan := ""
	if r.cfg.EnablePlanning {
		r.log.Info("entering planning phase")
		var err error
		plan, err = r.plan(ctx, userMessage)
		if err != nil {
			r.log.Warn("planning failed", zap.Error(err))
		} else {
			// Add plan to working memory
			planMsg := memory.Message{
				ID: uuid.New().String(), SessionID: r.sessionID, Role: memory.RoleSystem,
				Content: "[PLAN]\n" + plan, CreatedAt: time.Now(),
			}
			r.working.Append(planMsg)
		}
	}

	// Check if we need to flush memory before starting.
	// We'll perform one "proactive flush turn" if budget is tight.
	if r.shouldFlush() {
		r.doFlush(ctx, &totalUsage)
	}

	for iterations < r.cfg.MaxIterations {
		iterations++

		// Build the completion request from working memory.
		req := r.buildRequestV3(ctx, userMessage)

		r.log.Debug("running completion",
			zap.String("model", r.cfg.Model),
			zap.Int("messages", len(req.Messages)),
			zap.Int("iteration", iterations),
		)

		// Stream the response.
		stream, err := r.provider.Stream(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("agent: stream: %w", err)
		}

		resp, err := provider.CollectStream(ctx, stream)
		if err != nil {
			return nil, fmt.Errorf("agent: collect stream: %w", err)
		}

		totalUsage.PromptTokens += resp.Usage.PromptTokens
		totalUsage.CompletionTokens += resp.Usage.CompletionTokens
		totalUsage.TotalTokens += resp.Usage.TotalTokens

		// Append assistant message to working memory.
		assistantContent := resp.Text()
		toolCalls := resp.ToolCalls()

		if strings.TrimSpace(assistantContent) == "" && len(toolCalls) > 0 {
			assistantContent = "[Activating tools...]"
		}

		assistantMsg := memory.Message{
			ID:        uuid.New().String(),
			SessionID: r.sessionID,
			Role:      memory.RoleAssistant,
			Content:   assistantContent,
			Model:     r.cfg.Model,
			Tokens:    resp.Usage.CompletionTokens,
			CreatedAt: time.Now(),
		}
		r.working.Append(assistantMsg)
		if err := r.memory.Episodic.Save(ctx, assistantMsg); err != nil {
			r.log.Warn("episodic save failed", zap.Error(err))
		}

		// Markdown fallback parser: if the model dropped out of native tool schema
		// but printed bash instructions, we execute them autonomously anyway.
		if len(toolCalls) == 0 {
			toolCalls = extractMarkdownBash(assistantContent)
		}

		// If no tool calls, we're done.
		if len(toolCalls) == 0 || resp.FinishReason == provider.FinishReasonStop {
			// Compact working memory if over budget.
			r.working.Compact(r.working.TotalTokens())

			return &RunResult{
				SessionID:  r.sessionID,
				Response:   assistantContent,
				Iterations: iterations,
				Usage:      totalUsage,
			}, nil
		}

		// Execute tool calls and collect results.
		for _, tc := range toolCalls {
			result := r.executeTool(ctx, tc)

			toolResultMsg := memory.Message{
				ID:        uuid.New().String(),
				SessionID: r.sessionID,
				Role:      memory.RoleTool,
				Content:   result,
				CreatedAt: time.Now(),
			}
			r.working.Append(toolResultMsg)
			if err := r.memory.Episodic.Save(ctx, toolResultMsg); err != nil {
				r.log.Warn("episodic save failed", zap.Error(err))
			}
		}
	}

	// V3: Reflection Phase
	if r.cfg.EnableReflection {
		r.log.Info("entering reflection phase")
		critique, success, err := r.reflect(ctx, userMessage, plan)
		if err == nil {
			if !success && iterations < r.cfg.MaxIterations {
				r.log.Info("self-critique requested retry", zap.String("critique", critique))
				// Optionally append critique and keep going, but for now we just return
			}

			// Store any lessons learned
			if strings.Contains(critique, "<lesson_learned>") && r.cfg.EmbeddingModel != "" {
				lessonText := extractTag(critique, "lesson_learned")
				if lessonText != "" {
					emb, _ := r.provider.Embed(ctx, r.cfg.EmbeddingModel, userMessage)
					_ = r.memory.Semantic.SaveLesson(ctx, memory.Lesson{
						ID: uuid.New().String(), Query: userMessage, Insights: lessonText,
						Success: success, Embedding: emb, CreatedAt: time.Now(),
					})
				}
			}
		}
	}

	return nil, fmt.Errorf("agent: exceeded max iterations (%d)", r.cfg.MaxIterations)
}

func extractTag(text, tag string) string {
	startTag := "<" + tag + ">"
	endTag := "</" + tag + ">"
	start := strings.Index(text, startTag)
	if start == -1 {
		return ""
	}
	end := strings.Index(text, endTag)
	if end == -1 || end <= start+len(startTag) {
		return ""
	}
	return text[start+len(startTag) : end]
}

// extractMarkdownBash finds ```bash or ```sh blocks in the model's plaintext
// and wraps them into synthetic tool calls if the model failed to use the JSON schema.
func extractMarkdownBash(text string) []provider.ContentPart {
	var results []provider.ContentPart
	re := regexp.MustCompile(`(?s)\x60\x60\x60(?:bash|sh)\n(.*?)\x60\x60\x60`)
	matches := re.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		code := strings.TrimSpace(m[1])
		if code == "" {
			continue
		}
		inputMap := map[string]any{"command": code}
		results = append(results, provider.ContentPart{
			Type:      provider.ContentTypeToolUse,
			ToolUseID: "call_md_" + uuid.New().String()[:8],
			ToolName:  "bash",
			ToolInput: inputMap,
		})
	}
	return results
}

// executeTool runs a single tool call and returns the result string.
func (r *Runner) executeTool(ctx context.Context, tc provider.ContentPart) string {
	tool, ok := r.tools.Get(tc.ToolName)
	if !ok {
		r.log.Warn("tool not found", zap.String("tool", tc.ToolName))
		return fmt.Sprintf("Error: tool %q not found", tc.ToolName)
	}

	inputJSON, err := json.Marshal(tc.ToolInput)
	if err != nil {
		return fmt.Sprintf("Error marshalling tool input: %v", err)
	}

	r.log.Info("tool call",
		zap.String("tool", tc.ToolName),
		zap.String("input", truncate(string(inputJSON), 200)),
	)

	result, err := tool.Execute(ctx, inputJSON)
	if err != nil {
		r.log.Error("tool execution failed",
			zap.String("tool", tc.ToolName),
			zap.Error(err),
		)
		return fmt.Sprintf("Error: %v", err)
	}

	if strings.TrimSpace(result) == "" {
		result = "Command executed successfully with no output."
	}

	r.log.Info("tool result",
		zap.String("tool", tc.ToolName),
		zap.String("result", truncate(result, 200)),
	)
	return result
}

// buildRequest converts working memory messages to a provider request.
func (r *Runner) convertMessages(msgs []memory.Message) []provider.Message {
	providerMsgs := make([]provider.Message, 0, len(msgs))
	for _, m := range msgs {
		role := provider.Role(m.Role)
		providerMsgs = append(providerMsgs, provider.Message{
			Role: role,
			Content: []provider.ContentPart{
				{Type: provider.ContentTypeText, Text: m.Content},
			},
		})
	}
	return providerMsgs
}

func (r *Runner) buildRequestV3(ctx context.Context, query string) *provider.CompletionRequest {
	return &provider.CompletionRequest{
		Model:        r.cfg.Model,
		Messages:     r.convertMessages(r.working.Messages()),
		SystemPrompt: r.buildSystemPrompt(ctx, query),
		Tools:        r.tools.Definitions(),
		MaxTokens:    8096,
		Stream:       true,
	}
}

// buildRequest converts working memory messages to a provider request.
func (r *Runner) buildRequest() *provider.CompletionRequest {
	return &provider.CompletionRequest{
		Model:        r.cfg.Model,
		Messages:     r.convertMessages(r.working.Messages()),
		SystemPrompt: r.buildSystemPrompt(context.TODO(), ""), // default/empty
		Tools:        r.tools.Definitions(),
		MaxTokens:    8096,
		Stream:       true,
	}
}

func (r *Runner) buildSystemPrompt(ctx context.Context, query string) string {
	today := time.Now().Format("2006-01-02")
	ws := r.workspaceDir

	// ── Core identity ────────────────────────────────────────────────────────
	base := `You are AssistClaw — an autonomous coding and system agent. You have FULL ability to interact with the operating system, create files, run code, browse the web, and complete complex engineering tasks end-to-end.

## What you CAN do (and SHOULD do proactively)

You have the following built-in tools — USE THEM, don't just describe what to do:

| Tool | What it does |
|------|-------------|
| ` + "`bash`" + ` | Run ANY shell command: mkdir, git, npm/pip/go, run tests, compile, chmod, curl, etc. |
| ` + "`write_file`" + ` | Create or overwrite any file (source code, configs, scripts, manifests) |
| ` + "`read_file`" + ` | Read any file — source code, logs, configs, build output |
| ` + "`list_dir`" + ` | Browse directories recursively to understand project structure |
| ` + "`grep`" + ` | Search patterns across files (regex supported) |
| ` + "`web_fetch`" + ` | Fetch documentation, APIs, package READMEs, or any public URL |
| ` + "`browser_navigate`" + ` | Open a URL in a real browser session |
| ` + "`browser_screenshot`" + ` | Capture a screenshot of what the browser shows |
| ` + "`memory_search`" + ` | Search past conversations and indexed documents |

## How to handle requests

**IMPORTANT: When a user asks you to DO something, DO IT using your tools.**
**CRITICAL: Do NOT output conversational markdown code blocks (like ` + "`" + `bash ... ` + "`" + ` or ` + "`" + `json ... ` + "`" + `) expecting the user to run them or save them. You MUST use the ` + "`" + `write_file` + "`" + ` tool to save files and the ` + "`" + `bash` + "`" + ` tool to execute commands. If you output raw markdown blocks instead of calling the native JSON tools, your actions will not be executed!**

Examples:
- "Create a chrome extension" → use ` + "`write_file`" + ` to create manifest.json, content.js, etc. Use ` + "`bash`" + ` to zip it up.
- "Write a Python script" → use ` + "`write_file`" + ` to create the script, then ` + "`bash`" + ` to run it and show output.
- "Install numpy" → use ` + "`bash`" + ` with ` + "`pip install numpy`" + `.
- "Run the tests" → use ` + "`bash`" + ` with the test command.
- "Check if it works" → use ` + "`bash`" + ` to execute the code and return actual output.

## Coding task workflow

1. **Understand** the request (ask one clarifying question if truly ambiguous, then act)
2. **Explore** with ` + "`list_dir`" + ` or ` + "`read_file`" + ` if working in an existing project
3. **Implement** with ` + "`write_file`" + ` (create files) and/or ` + "`bash`" + ` (run commands)
4. **Verify** by running the code with ` + "`bash`" + ` and including the real output in your reply
5. **Report** what you created/changed and where it lives

## Workspace
Your persistent workspace is at: ` + ws + `
- Global memory: ` + ws + `/MEMORY.md
- Daily log: ` + ws + `/memory/` + today + `.md
Use ` + "`write_file`" + ` to record important insights to these files.`

	var parts []string
	parts = append(parts, base)

	// Channel-specific tone adjustments (after identity, not replacing it)
	if r.channelID == "whatsapp" {
		parts = append(parts, "CHANNEL: WhatsApp. Keep replies concise. Use bullet points. Show only the most important output snippets — not full file contents. If you created files, say where they are and show a 3-5 line preview.")
	}

	if r.cfg.SystemPrompt != "" {
		parts = append(parts, r.cfg.SystemPrompt)
	}

	// Corrective Memory (Lessons Learned from past tasks)
	if query != "" && r.cfg.EmbeddingModel != "" {
		emb, err := r.provider.Embed(ctx, r.cfg.EmbeddingModel, query)
		if err == nil {
			lessons, err := r.memory.Semantic.SearchLessons(ctx, emb, 3)
			if err == nil && len(lessons) > 0 {
				var sb strings.Builder
				sb.WriteString("\n## Past Task Insights\n")
				for _, l := range lessons {
					sb.WriteString(fmt.Sprintf("- Task: %s\n  Insight: %s\n", l.Query, l.Insights))
				}
				parts = append(parts, sb.String())
			}
		}
	}

	if r.cfg.ActiveSkillsContext != "" {
		parts = append(parts, r.cfg.ActiveSkillsContext)
	}

	return strings.Join(parts, "\n\n")
}

// ─────────────────────────────────────────────
// Stream runner (for interactive CLI use)
// ─────────────────────────────────────────────

// StreamHandler receives streaming events for real-time display.
type StreamHandler interface {
	OnToken(token string)
	OnToolCall(name string, input json.RawMessage)
	OnToolResult(name string, result string)
	OnDone(result *RunResult)
	OnError(err error)
}

// RunStream runs the agent loop and calls handler methods as events occur.
// Designed for live terminal interaction.
func (r *Runner) RunStream(ctx context.Context, userMessage string, handler StreamHandler) {
	// Append user message.
	userMsg := memory.Message{
		ID:        uuid.New().String(),
		SessionID: r.sessionID,
		Role:      memory.RoleUser,
		Content:   userMessage,
		CreatedAt: time.Now(),
	}
	r.working.Append(userMsg)
	_ = r.memory.Episodic.Save(ctx, userMsg)

	var totalUsage provider.TokenUsage
	var fullResponse strings.Builder
	iterations := 0

	// V3: Planning Phase
	plan := ""
	if r.cfg.EnablePlanning {
		r.log.Info("entering planning phase (stream)")
		handler.OnToken("🤔 **Planning...**\n")
		var err error
		plan, err = r.plan(ctx, userMessage)
		if err != nil {
			r.log.Warn("planning failed", zap.Error(err))
		} else {
			// Add plan to working memory
			planMsg := memory.Message{
				ID: uuid.New().String(), SessionID: r.sessionID, Role: memory.RoleSystem,
				Content: "[PLAN]\n" + plan, CreatedAt: time.Now(),
			}
			r.working.Append(planMsg)
			handler.OnToken("\n<details>\n<summary>Execution Plan</summary>\n\n" + plan + "\n\n</details>\n\n")
		}
	}

	// Pre-compaction flush for streaming
	if r.shouldFlush() {
		r.doFlushStream(ctx, handler, &totalUsage)
	}

	for iterations < r.cfg.MaxIterations {
		iterations++
		fullResponse.Reset()

		// V3: Use buildRequestV3 with query context
		stream, err := r.provider.Stream(ctx, r.buildRequestV3(ctx, userMessage))
		if err != nil {
			handler.OnError(fmt.Errorf("agent: stream: %w", err))
			return
		}

		var toolCalls []provider.ContentPart
		var finishReason provider.FinishReason

		for event := range stream {
			switch event.Type {
			case provider.StreamEventText:
				handler.OnToken(event.Text)
				fullResponse.WriteString(event.Text)
			case provider.StreamEventToolUse:
				if event.ToolUse != nil {
					toolCalls = append(toolCalls, *event.ToolUse)
				}
			case provider.StreamEventDone:
				finishReason = event.FinishReason
				if event.Usage != nil {
					totalUsage.PromptTokens += event.Usage.PromptTokens
					totalUsage.CompletionTokens += event.Usage.CompletionTokens
					totalUsage.TotalTokens += event.Usage.TotalTokens
				}
			case provider.StreamEventError:
				handler.OnError(event.Err)
				return
			}
		}

		assistantContent := fullResponse.String()
		if strings.TrimSpace(assistantContent) == "" && len(toolCalls) > 0 {
			assistantContent = "[Activating tools...]"
		}

		assistantMsg := memory.Message{
			ID: uuid.New().String(), SessionID: r.sessionID, Role: memory.RoleAssistant,
			Content: assistantContent, Model: r.cfg.Model,
			Tokens: totalUsage.CompletionTokens, CreatedAt: time.Now(),
		}
		r.working.Append(assistantMsg)
		_ = r.memory.Episodic.Save(ctx, assistantMsg)

		if len(toolCalls) == 0 || finishReason == provider.FinishReasonStop {
			handler.OnDone(&RunResult{
				SessionID: r.sessionID, Response: fullResponse.String(),
				Iterations: iterations, Usage: totalUsage,
			})
			return
		}

		for _, tc := range toolCalls {
			inputJSON, _ := json.Marshal(tc.ToolInput)
			handler.OnToolCall(tc.ToolName, inputJSON)
			result := r.executeTool(ctx, tc)
			handler.OnToolResult(tc.ToolName, result)

			toolMsg := memory.Message{
				ID: uuid.New().String(), SessionID: r.sessionID, Role: memory.RoleTool,
				Content: result, CreatedAt: time.Now(),
			}
			r.working.Append(toolMsg)
			_ = r.memory.Episodic.Save(ctx, toolMsg)
		}
	}

	// V3: Reflection Phase
	if r.cfg.EnableReflection {
		r.log.Info("entering reflection phase (stream)")
		handler.OnToken("\n\n🧐 **Reflecting...**\n")
		critique, success, err := r.reflect(ctx, userMessage, plan)
		if err == nil {
			handler.OnToken("\n<details>\n<summary>Self-Critique</summary>\n\n" + critique + "\n\n</details>\n")
			if !success && iterations < r.cfg.MaxIterations {
				r.log.Info("self-critique requested retry", zap.String("critique", critique))
			}

			// Store any lessons learned
			if strings.Contains(critique, "<lesson_learned>") && r.cfg.EmbeddingModel != "" {
				lessonText := extractTag(critique, "lesson_learned")
				if lessonText != "" {
					emb, _ := r.provider.Embed(ctx, r.cfg.EmbeddingModel, userMessage)
					_ = r.memory.Semantic.SaveLesson(ctx, memory.Lesson{
						ID: uuid.New().String(), Query: userMessage, Insights: lessonText,
						Success: success, Embedding: emb, CreatedAt: time.Now(),
					})
				}
			}
		}
	}

	handler.OnError(fmt.Errorf("agent: exceeded max iterations (%d)", r.cfg.MaxIterations))
}

// HandleChannelMessage is a background message handler for messaging channels.
func (r *Runner) HandleChannelMessage(ctx context.Context, msg channels.Message, replyFn channels.StreamingReplyFunc) {
	r.log.Info("inbound message",
		zap.String("channel", msg.ChannelID),
		zap.String("session", msg.SessionID),
		zap.String("text", truncate(msg.Text, 100)),
	)

	// Note: In a production system, we would resolve a dedicated Runner instance per SessionID
	// to maintain separate working memories. For now, we share the runner's session logic.

	// Use a dedicated runner instance for this session ID and channel
	sessionRunner := r.WithSession(msg.SessionID)
	sessionRunner.channelID = msg.ChannelID

	handler := &channelStreamHandler{
		replyFn: replyFn,
	}

	sessionRunner.RunStream(ctx, msg.Text, handler)
}

// channelStreamHandler routes agent tokens back to a messaging channel.
type channelStreamHandler struct {
	replyFn channels.StreamingReplyFunc
}

func (h *channelStreamHandler) OnToken(token string) {
	_ = h.replyFn(token)
}

func (h *channelStreamHandler) OnToolCall(name string, _ json.RawMessage) {
	// Optional: send status update to channel
}

func (h *channelStreamHandler) OnToolResult(name string, _ string) {
	// Optional: send status update to channel
}

func (h *channelStreamHandler) OnDone(_ *RunResult) {
	_ = h.replyFn("") // signal done
}

func (h *channelStreamHandler) OnError(err error) {
	_ = h.replyFn(fmt.Sprintf("\n[Error: %v]", err))
}

func (r *Runner) shouldFlush() bool {
	budget := r.working.MaxTokens()
	current := r.working.TotalTokens()
	// Flush if we are at 80% capacity
	return current > int(float64(budget)*0.8)
}

func (r *Runner) doFlush(ctx context.Context, usage *provider.TokenUsage) {
	date := time.Now().Format("2006-01-02")
	prompt := fmt.Sprintf("MEMORY NEAR CAPACITY. Store important info to MEMORY.md or memory/%s.md now if needed. Reply with [SILENT] if nothing to store.", date)

	// Inject a system-like user message for the flush turn
	flushMsg := memory.Message{
		ID: uuid.New().String(), SessionID: r.sessionID, Role: memory.RoleUser,
		Content: prompt, CreatedAt: time.Now(),
	}
	r.working.Append(flushMsg)

	req := r.buildRequest()
	stream, err := r.provider.Stream(ctx, req)
	if err != nil {
		r.log.Warn("memory flush turn failed", zap.Error(err))
		return
	}

	resp, err := provider.CollectStream(ctx, stream)
	if err != nil {
		r.log.Warn("memory flush turn collect failed", zap.Error(err))
		return
	}

	usage.PromptTokens += resp.Usage.PromptTokens
	usage.CompletionTokens += resp.Usage.CompletionTokens
	usage.TotalTokens += resp.Usage.TotalTokens

	assistantContent := resp.Text()
	if strings.TrimSpace(assistantContent) == "" && len(resp.ToolCalls()) > 0 {
		assistantContent = "[Activating tools...]"
	}

	assistantMsg := memory.Message{
		ID: uuid.New().String(), SessionID: r.sessionID, Role: memory.RoleAssistant,
		Content: assistantContent, Model: r.cfg.Model, Tokens: resp.Usage.CompletionTokens, CreatedAt: time.Now(),
	}
	r.working.Append(assistantMsg)

	// Execute any tools (like write_file) requested during flush.
	for _, tc := range resp.ToolCalls() {
		result := r.executeTool(ctx, tc)
		toolMsg := memory.Message{
			ID: uuid.New().String(), SessionID: r.sessionID, Role: memory.RoleTool,
			Content: result, CreatedAt: time.Now(),
		}
		r.working.Append(toolMsg)
	}
}

func (r *Runner) doFlushStream(ctx context.Context, handler StreamHandler, usage *provider.TokenUsage) {
	date := time.Now().Format("2006-01-02")
	prompt := fmt.Sprintf("MEMORY NEAR CAPACITY. Store important info to MEMORY.md or memory/%s.md now if needed. Reply with [SILENT] if nothing to store.", date)

	flushMsg := memory.Message{
		ID: uuid.New().String(), SessionID: r.sessionID, Role: memory.RoleUser,
		Content: prompt, CreatedAt: time.Now(),
	}
	r.working.Append(flushMsg)

	stream, err := r.provider.Stream(ctx, r.buildRequest())
	if err != nil {
		handler.OnError(fmt.Errorf("agent: flush stream: %w", err))
		return
	}

	var fullResponse strings.Builder
	var toolCalls []provider.ContentPart

	for event := range stream {
		switch event.Type {
		case provider.StreamEventText:
			handler.OnToken(event.Text)
			fullResponse.WriteString(event.Text)
		case provider.StreamEventToolUse:
			if event.ToolUse != nil {
				toolCalls = append(toolCalls, *event.ToolUse)
			}
		case provider.StreamEventDone:
			if event.Usage != nil {
				usage.PromptTokens += event.Usage.PromptTokens
				usage.CompletionTokens += event.Usage.CompletionTokens
				usage.TotalTokens += event.Usage.TotalTokens
			}
		case provider.StreamEventError:
			handler.OnError(event.Err)
			return
		}
	}

	assistantContent := fullResponse.String()
	if strings.TrimSpace(assistantContent) == "" && len(toolCalls) > 0 {
		assistantContent = "[Activating tools...]"
	}

	assistantMsg := memory.Message{
		ID: uuid.New().String(), SessionID: r.sessionID, Role: memory.RoleAssistant,
		Content: assistantContent, Model: r.cfg.Model,
		Tokens: usage.CompletionTokens, CreatedAt: time.Now(),
	}
	r.working.Append(assistantMsg)

	for _, tc := range toolCalls {
		inputJSON, _ := json.Marshal(tc.ToolInput)
		handler.OnToolCall(tc.ToolName, inputJSON)
		result := r.executeTool(ctx, tc)
		handler.OnToolResult(tc.ToolName, result)

		toolMsg := memory.Message{
			ID: uuid.New().String(), SessionID: r.sessionID, Role: memory.RoleTool,
			Content: result, CreatedAt: time.Now(),
		}
		r.working.Append(toolMsg)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
