package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/assistclaw/assistclaw/internal/memory"
	"github.com/assistclaw/assistclaw/internal/provider"
)

// RunAutonomous executes the agent loop continuously until the LLM explicitly calls the
// `finish_task` tool, signaling that the high-level goal has been reached.
func (r *Runner) RunAutonomous(ctx context.Context, goal string) (*RunResult, error) {
	r.inflight.Add(1)
	defer r.inflight.Done()
	if goal != "" {
		// Append overarching goal to working memory.
		goalMsg := memory.Message{
			ID: uuid.New().String(), SessionID: r.sessionID, Role: memory.RoleSystem,
			Content:   "AUTONOMOUS GOAL: " + goal + "\n\nWork continuously to achieve this goal. When you are completely finished, call the `finish_task` tool.",
			CreatedAt: time.Now(),
		}
		r.working.Append(goalMsg)
		_ = r.memory.Episodic.Save(ctx, goalMsg)
	}

	var totalUsage provider.TokenUsage
	iterations := 0
	maxAutoIterations := 500 // higher limit for continuous mode

	// Check if we need to flush memory before starting.
	if r.shouldFlush() {
		r.doFlush(ctx, &totalUsage)
	}

	// Wait before loops to avoid API rate limits running unchecked
	loopDelay := 2 * time.Second

	// Upfront plan for long-horizon work (same as interactive Run).
	if r.cfg.EnablePlanning && goal != "" {
		r.log.Info("autonomous: planning phase")
		plan, err := r.plan(ctx, goal)
		if err != nil {
			r.log.Warn("autonomous: planning failed", zap.Error(err))
		} else if strings.TrimSpace(plan) != "" {
			planMsg := memory.Message{
				ID: uuid.New().String(), SessionID: r.sessionID, Role: memory.RoleSystem,
				Content:   "[PLAN]\n" + plan,
				CreatedAt: time.Now(),
			}
			r.working.Append(planMsg)
			_ = r.memory.Episodic.Save(ctx, planMsg)
		}
	}

	for iterations < maxAutoIterations {
		iterations++

		// Periodic checkpoint so the model re-orients on very long autonomous runs.
		if iterations > 1 && iterations%25 == 0 {
			checkMsg := memory.Message{
				ID:        uuid.New().String(),
				SessionID: r.sessionID,
				Role:      memory.RoleSystem,
				Content:   "[AUTONOMOUS CHECKPOINT] Assess progress toward the AUTONOMOUS GOAL. If stuck, change approach. Continue with tools, or call finish_task when fully done.",
				CreatedAt: time.Now(),
			}
			r.working.Append(checkMsg)
			_ = r.memory.Episodic.Save(ctx, checkMsg)
		}

		req := r.buildRequestV3(ctx, goal)

		r.log.Debug("running autonomous completion",
			zap.String("model", r.cfg.Model),
			zap.Int("messages", len(req.Messages)),
			zap.Int("iteration", iterations),
		)

		stream, err := r.provider.Stream(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("agent: autonomous stream: %w", err)
		}

		resp, err := provider.CollectStream(ctx, stream)
		if err != nil {
			return nil, fmt.Errorf("agent: autonomous collect stream: %w", err)
		}

		totalUsage.PromptTokens += resp.Usage.PromptTokens
		totalUsage.CompletionTokens += resp.Usage.CompletionTokens
		totalUsage.TotalTokens += resp.Usage.TotalTokens

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
			Parts:     toolCalls,
			Model:     r.cfg.Model,
			Tokens:    resp.Usage.CompletionTokens,
			CreatedAt: time.Now(),
		}
		r.working.Append(assistantMsg)
		_ = r.memory.Episodic.Save(ctx, assistantMsg)

		// Markdown fallback parser: if the model dropped out of native tool schema
		if len(toolCalls) == 0 {
			toolCalls = extractMarkdownBash(assistantContent)
		}

		if len(toolCalls) == 0 && resp.FinishReason == provider.FinishReasonStop {
			// In autonomous mode, we don't exit if no tools are called. We nudge the agent.
			// UNLESS it explicitly requested to finish. (But if no tool calls were made, it didn't call finish_task).
			nudgeContent := "Please continue working towards the goal. What is the next step? Keep using tools. If you are entirely finished, you MUST call the `finish_task` tool."
			nudgeMsg := memory.Message{
				ID:        uuid.New().String(),
				SessionID: r.sessionID,
				Role:      memory.RoleUser,
				Content:   nudgeContent,
				CreatedAt: time.Now(),
			}
			r.working.Append(nudgeMsg)
			_ = r.memory.Episodic.Save(ctx, nudgeMsg)
			time.Sleep(loopDelay)
			continue
		}

		// Execute tool calls
		finished := false
		for _, tc := range toolCalls {
			if tc.ToolName == "finish_task" {
				finished = true
			}

			result := r.executeTool(ctx, tc, nil)

			toolResultMsg := memory.Message{
				ID:        uuid.New().String(),
				SessionID: r.sessionID,
				Role:      memory.RoleTool,
				Content:   result,
				Parts: []provider.ContentPart{
					{
						Type:              provider.ContentTypeToolResult,
						ToolResultID:      tc.ToolUseID,
						ToolResultContent: result,
					},
				},
				CreatedAt: time.Now(),
			}
			r.working.Append(toolResultMsg)
			_ = r.memory.Episodic.Save(ctx, toolResultMsg)
		}

		// If finish_task was called, we gracefully exit the autonomous loop
		if finished {
			// Compact working memory if over budget.
			r.working.Compact(r.working.TotalTokens())

			return &RunResult{
				SessionID:  r.sessionID,
				Response:   "Autonomous goal completed.",
				Iterations: iterations,
				Usage:      totalUsage,
			}, nil
		}

		// Periodically check flush
		if r.shouldFlush() {
			r.doFlush(ctx, &totalUsage)
		}

		time.Sleep(loopDelay)
	}

	return nil, fmt.Errorf("agent: exceeded max autonomous iterations (%d)", maxAutoIterations)
}
