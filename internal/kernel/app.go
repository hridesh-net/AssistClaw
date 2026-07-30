package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/assistclaw/assistclaw/internal/agent"
	"github.com/assistclaw/assistclaw/internal/awareness"
	"github.com/assistclaw/assistclaw/internal/config"
	"github.com/assistclaw/assistclaw/internal/cron"
	"github.com/assistclaw/assistclaw/internal/embeddings"
	"github.com/assistclaw/assistclaw/internal/extensions"
	"github.com/assistclaw/assistclaw/internal/graph"
	"github.com/assistclaw/assistclaw/internal/localintel"
	"github.com/assistclaw/assistclaw/internal/mcp"
	"github.com/assistclaw/assistclaw/internal/memory"
	"github.com/assistclaw/assistclaw/internal/mempalace"
	"github.com/assistclaw/assistclaw/internal/provider"
	"github.com/assistclaw/assistclaw/internal/security"
	"github.com/assistclaw/assistclaw/internal/skills"
	"github.com/assistclaw/assistclaw/internal/subagents"
	"github.com/assistclaw/assistclaw/internal/system"
	"github.com/assistclaw/assistclaw/internal/tools"
)

// App is the constructed AssistClaw application: every long-lived subsystem the
// agent needs, wired together by Build. It is the output of the composition
// root. Callers run the agent (single message, autonomous, serve) using its
// fields, then call Close to release resources in reverse construction order.
type App struct {
	Cfg      *config.Config
	Log      *zap.Logger
	Mem      *memory.Manager
	Runner   *agent.Runner
	Provider provider.Provider // failover-wrapped resolved provider
	ModelID  string
	// ChannelSenders is shared by reference with the message-sending tools built
	// during construction; the serve layer populates it as channels come up.
	ChannelSenders map[string]tools.ChannelSender
	Aware          *awareness.Store
	Cron           *cron.Daemon

	externalMCP []*mcp.Client
}

// BuildOptions carries the per-invocation inputs construction needs that do not
// come from config (the requested model and sensitive-skill allow-lists).
type BuildOptions struct {
	Model                   string
	AllowSensitiveSkills    []string
	AllowAllSensitiveSkills bool
}

// agentPlanningEnabled defaults to true when unset (upfront planning on).
func agentPlanningEnabled(c *config.Config) bool {
	if c.Agent.Planning == nil {
		return true
	}
	return *c.Agent.Planning
}

// agentReflectionEnabled defaults to false when unset (extra LLM call; opt-in).
func agentReflectionEnabled(c *config.Config) bool {
	if c.Agent.Reflection == nil {
		return false
	}
	return *c.Agent.Reflection
}

// Close releases App resources in reverse construction order. Safe to call on a
// partially-built App (all fields nil-checked) so Build can use it for cleanup
// on an error path.
func (a *App) Close() error {
	if a == nil {
		return nil
	}
	if a.Cron != nil {
		a.Cron.Stop()
	}
	for i := len(a.externalMCP) - 1; i >= 0; i-- {
		_ = a.externalMCP[i].Close()
	}
	if a.Mem != nil {
		_ = a.Mem.Close()
	}
	return nil
}

// Build constructs all agent subsystems from config: provider/embedder
// registries, the three memory tiers, model resolution, skills, the tool
// registry and catalog, external MCP tools, the failover provider, the runner,
// security guardrail/audit, sub-agents, awareness, and the cron daemon. It is a
// straight-line move of what used to be the first ~450 lines of cmd's runAgent;
// behavior is unchanged. The caller owns the returned App and must Close it.
func Build(ctx context.Context, cfg *config.Config, log *zap.Logger, opts BuildOptions) (*App, error) {
	app := &App{Cfg: cfg, Log: log}

	// Boot all subsystems
	reg := provider.NewRegistry()
	if err := RegisterProviders(ctx, cfg, reg, log); err != nil {
		log.Warn("some providers failed to register", zap.Error(err))
	}

	embedReg := embeddings.NewRegistry()
	RegisterEmbedders(ctx, cfg, embedReg, log)

	// Ensure memory dirs exist
	if err := os.MkdirAll(filepath.Dir(cfg.Memory.EpisodicDBPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Memory.SemanticDBPath), 0o755); err != nil {
		return nil, err
	}
	dims := 1536 // default for OpenAI small
	if e, ok := embedReg.Default(); ok {
		models, _ := e.ListModels(ctx)
		if len(models) > 0 {
			dims = models[0].Dimensions
		}
	}
	memMgr, err := memory.NewManager(memory.ManagerConfig{
		WorkingTokenBudget:  cfg.Memory.WorkingTokenBudget,
		EpisodicDBPath:      cfg.Memory.EpisodicDBPath,
		SemanticDBPath:      cfg.Memory.SemanticDBPath,
		EmbeddingDimensions: dims,
		ChunkSize:           cfg.Memory.Mining.ChunkSize,
		ChunkOverlap:        cfg.Memory.Mining.ChunkOverlap,
	})
	if err != nil {
		return nil, fmt.Errorf("memory init: %w", err)
	}
	app.Mem = memMgr

	// Start Markdown memory watcher/synchronizer
	go func() {
		if err := memMgr.Watch(ctx, embedReg, cfg.StateDir); err != nil {
			log.Warn("memory watcher failed", zap.Error(err))
		}
	}()

	// Resolve model
	resolvedModel := opts.Model
	if resolvedModel == "" {
		resolvedModel = cfg.Routing.Default
	}
	if resolvedModel == "" {
		// Auto-select first available provider
		for _, p := range reg.All() {
			models, _ := p.ListModels(ctx)
			if len(models) > 0 {
				resolvedModel = p.Name() + "/" + models[0].ID
				break
			}
		}
	}
	if resolvedModel == "" {
		app.Close()
		return nil, fmt.Errorf("no model configured — set routing.default in config or use --model")
	}

	p, modelInfo, err := reg.ResolveModel(ctx, resolvedModel)
	if err != nil {
		app.Close()
		return nil, fmt.Errorf("resolve model %q: %w", resolvedModel, err)
	}
	log.Info("using model", zap.String("model", modelInfo.ID), zap.String("provider", p.Name()))

	// Extract bundled skills on first run
	bundledDir := filepath.Join(cfg.StateDir, "skills", "bundled")
	customDir := filepath.Join(cfg.StateDir, "skills", "custom")
	if err := ExtractBundledSkills(bundledDir); err != nil {
		log.Warn("failed to extract bundled skills", zap.Error(err))
	}

	// Load Skills — respects enabled_skills if set, else loads all from custom dir
	skillReg := skills.NewRegistry()
	if err := skillReg.LoadAll(ctx, customDir); err != nil {
		log.Warn("failed to load skills", zap.Error(err))
	}

	// Determine active skill names
	activeSkillNames := cfg.Agent.EnabledSkills
	if len(activeSkillNames) == 0 {
		// No explicit list = all installed custom skills are active
		for _, s := range skillReg.List() {
			activeSkillNames = append(activeSkillNames, s.Name)
		}
	}

	// Build tool registry
	toolReg := agent.NewToolRegistry()

	// Build the sensitive-skill approver: union of YAML allow-list, CLI
	// allow-list, and the "trust everything" CLI escape hatch. Sensitive
	// skills not in this set will refuse to execute at call time.
	allowedSensitive := map[string]bool{}
	for _, n := range cfg.Agent.EnabledSensitiveSkills {
		allowedSensitive[strings.TrimSpace(n)] = true
	}
	for _, n := range opts.AllowSensitiveSkills {
		allowedSensitive[strings.TrimSpace(n)] = true
	}
	allowAllSensitive := opts.AllowAllSensitiveSkills
	approver := func(skillName, _ string) bool {
		return allowAllSensitive || allowedSensitive[skillName]
	}

	// Register tools from skills
	for _, s := range skillReg.List() {
		skillTools := skills.ConvertTools(&s, cfg.Agent.SkillsDir, approver) // Simplification: assuming all tools relative to skills dir
		for _, t := range skillTools {
			toolReg.Register(t)
		}
	}

	if cfg.Memory.MemPalace.ManagedVenv && cfg.Memory.MemPalace.AutoStart {
		log.Info("mempalace: ensuring managed venv (first run may download PyPI packages)")
		if err := mempalace.Ensure(ctx, mempalace.EnsureOptions{
			StateDir:        cfg.StateDir,
			BootstrapPython: cfg.Memory.MemPalace.BootstrapPython,
			Progress:        os.Stderr,
			Log:             log,
		}); err != nil {
			app.Close()
			return nil, fmt.Errorf("mempalace managed venv: %w", err)
		}
	}

	mcpClientList, mempalaceAuto := EffectiveMCPClients(cfg)
	if mempalaceAuto {
		msg := "mcp: MemPalace auto-start (stdio child process)"
		if cfg.Memory.MemPalace.ManagedVenv {
			msg = "mcp: MemPalace auto-start (managed venv + stdio child)"
		}
		log.Info(msg,
			zap.String("client", strings.TrimSpace(cfg.Memory.MemPalace.MCPClientName)),
			zap.String("python", strings.TrimSpace(cfg.Memory.MemPalace.PythonExecutable)),
		)
	}
	mcpCfgs := MCPClientConfigsFromYAML(mcpClientList)
	if len(mcpCfgs) > 0 {
		app.externalMCP = mcp.RegisterExternalMCPTools(ctx, mcpCfgs, skillReg, toolReg, nil, log)
	}

	// Virtual MCP skills are registered after the initial activeSkillNames pass; include them so
	// BuildContext, skill_graph_index, and read_skill_node see custom MCP servers alongside disk skills.
	activeSkillNames = AugmentActiveSkillsWithMCP(skillReg, activeSkillNames)
	skillsCtx := skillReg.BuildContext(activeSkillNames)

	memSearchFn := func(searchCtx context.Context, query string, limit int) ([]string, error) {
		var out []string
		router := memory.NewHeuristicPalaceRouter()
		route := router.Route(query)
		shouldRoute := cfg.Agent.Palace.Enabled && !cfg.Agent.Palace.ShadowOnly && cfg.Agent.Palace.MemorySearchRouting

		// 1. Episodic Search (Full-Text)
		msgs, err := memMgr.Episodic.Search(searchCtx, query, limit)
		if err == nil {
			for _, m := range msgs {
				out = append(out, fmt.Sprintf("[episodic] [%s] %s: %s", m.CreatedAt.Format("2006-01-02 15:04"), m.Role, m.Content))
			}
		}

		// 2. Semantic Search (Vector)
		if vec, err := embedReg.EmbedQuery(searchCtx, query); err == nil {
			docs, err := memMgr.Semantic.SearchWithModel(searchCtx, vec, limit)
			if err == nil {
				if shouldRoute && !route.IsZero() {
					filtered := make([]memory.Document, 0, len(docs))
					for _, d := range docs {
						if route.MatchesDocument(d) {
							filtered = append(filtered, d)
						}
					}
					if len(filtered) > 0 {
						docs = filtered
					} else if !cfg.Agent.Palace.FailOpen {
						docs = nil
					}
				}
				var docPaths []string
				for _, d := range docs {
					docPaths = append(docPaths, d.Source)
					out = append(out, fmt.Sprintf("[semantic] [score:%.2f] [%s / %s] source=%s taxonomy=%s/%s/%s: %s", d.Score, d.Model, d.CreatedAt.Format("2006-01-02 15:04"), d.Source, d.Palace, d.Wing, d.Room, d.Content))
				}

				// QueryWeaver Logic: Discover bridges between matched skill nodes
				if bridges := skillReg.FindBridges(docPaths); len(bridges) > 0 {
					for _, b := range bridges {
						out = append(out, fmt.Sprintf("[semantic] [bridge] source=%s: %s", b.FilePath, b.Instructions))
					}
				}
			}
		}

		if cfg.Memory.MemPalace.Enabled && cfg.Memory.MemPalace.InjectIntoMemorySearch {
			clientName := strings.TrimSpace(cfg.Memory.MemPalace.MCPClientName)
			if clientName == "" {
				clientName = "mempalace"
			}
			toolName := "mcp:" + clientName + ":mempalace_search"
			if t, ok := toolReg.Get(toolName); ok {
				mpLimit := limit
				if cfg.Memory.MemPalace.SearchLimit > 0 {
					mpLimit = cfg.Memory.MemPalace.SearchLimit
				}
				input, err := json.Marshal(map[string]any{
					"query": query,
					"limit": mpLimit,
				})
				if err == nil {
					if text, err := t.Execute(searchCtx, input); err != nil {
						log.Debug("mempalace_search delegate failed", zap.String("tool", toolName), zap.Error(err))
					} else if strings.TrimSpace(text) != "" {
						out = append(out, "[mempalace] "+text)
					}
				}
			} else {
				log.Warn("memory.mempalace.inject_into_memory_search is true but MCP tool is missing",
					zap.String("expected_tool", toolName),
					zap.String("hint", "enable memory.mempalace.managed_venv + auto_start, or run: assistclaw mempalace setup"),
				)
			}
		}

		return out, nil
	}
	memSnippetFn := func(snippetCtx context.Context, source string, startLine, endLine int) (string, error) {
		// If source doesn't exist, try resolving it relative to workspace
		path := source
		if _, err := os.Stat(path); os.IsNotExist(err) {
			path = filepath.Join(cfg.StateDir, source) // cfg.StateDir is workspace dir for now
		}
		return memMgr.Semantic.GetSnippet(snippetCtx, path, startLine, endLine)
	}
	channelSenders := map[string]tools.ChannelSender{}
	for _, t := range tools.Default(memSearchFn, memSnippetFn, memMgr.Episodic, p, modelInfo.ID, channelSenders, cfg.StateDir) {
		if tool, ok := t.(agent.Tool); ok {
			toolReg.Register(tool)
		}
	}
	// Build the graph-based tool catalog for per-request token-efficient selection.
	toolGraph := graph.NewToolGraph()
	catalog := tools.NewCatalog(toolReg, toolGraph)

	// Register find_tools (the Anthropic-pattern tool discovery tool).
	toolReg.Register(tools.FindToolsTool{Catalog: catalog})

	// Register skill_graph_index (on-demand skill node discovery).
	// activeSkillNames is already populated above (cfg.Agent.EnabledSkills or all loaded skills).
	toolReg.Register(&skills.SkillGraphIndexTool{
		Registry:     skillReg,
		ActiveSkills: activeSkillNames,
	})

	// Also register read_skill_node here (previously registered separately).
	toolReg.Register(skills.NewReadSkillNodeTool(skillReg))

	// Register repair_skill (auto-installation of missing dependencies).
	toolReg.Register(&skills.RepairSkillTool{Registry: skillReg})

	// Proactive self-healing: repair all enabled skills if they have missing dependencies.
	_ = skillReg.RepairAllEnabled(ctx, activeSkillNames)

	// Rebuild catalog to include all newly registered tools (find_tools, skill_graph_index, read_skill_node).
	catalog = tools.NewCatalog(toolReg, toolGraph)

	// Derive provider name for capability detection from the resolved model ID.
	// e.g. "anthropic/claude-opus-4" → "anthropic", "claude-opus-4" → "" (uses default caps)
	providerNameForCaps := ""
	if modelInfo.ID != "" {
		if idx := strings.Index(modelInfo.ID, "/"); idx > 0 {
			providerNameForCaps = strings.ToLower(modelInfo.ID[:idx])
		}
	}

	hw, _ := system.Detect(ctx)
	extPrompt := ""
	if cfg.Extensions.Enabled && len(cfg.Extensions.PromptFiles) > 0 {
		extPrompt = extensions.PromptAppendix(cfg.StateDir, cfg.Extensions.PromptFiles)
	}
	localIntelCache := strings.TrimSpace(cfg.Agent.LocalIntel.CacheDir)
	if localIntelCache == "" {
		localIntelCache = filepath.Join(cfg.StateDir, "localintel")
	}

	// Wrap all registered providers in a failover layer with circuit breakers.
	// The originally resolved provider becomes the first preferred primary.
	var primaries []provider.Provider
	primaries = append(primaries, p)
	for _, rp := range reg.All() {
		if rp.Name() != p.Name() {
			primaries = append(primaries, rp)
		}
	}
	// Build local gemma fallback if configured.
	var fallback provider.Provider
	if cfg.Agent.LocalIntel.Enabled && cfg.Agent.LocalIntel.GGUFPath != "" {
		liEng, liErr := localintel.Open(localintel.Options{
			CacheDir: localIntelCache,
			GGUFPath: cfg.Agent.LocalIntel.GGUFPath,
		})
		if liErr == nil && liEng.Available() {
			fallback = provider.NewLocalIntelProvider(liEng, "local/gemma", cfg.Agent.LocalIntel.SystemPrompt, cfg.Agent.LocalIntel.MaxTokens)
			log.Info("local intel fallback available", zap.String("model", "local/gemma"))
		} else if liErr != nil {
			log.Warn("local intel fallback unavailable", zap.Error(liErr))
		}
	}
	if len(primaries) > 1 || fallback != nil {
		p = provider.NewFailoverProvider(primaries, fallback, log)
		log.Info("provider failover enabled", zap.Int("primaries", len(primaries)), zap.Bool("fallback", fallback != nil))
	}

	runner := agent.NewRunner(agent.Config{
		MaxIterations:         cfg.Agent.MaxIterations,
		Model:                 modelInfo.ID,
		ActiveSkillsContext:   skillsCtx,
		ProviderName:          providerNameForCaps,
		ToolsProfile:          cfg.Security.Profile,
		EnablePlanning:        agentPlanningEnabled(cfg),
		EnableReflection:      agentReflectionEnabled(cfg),
		GatewayPublicBaseURL:  cfg.PublicGatewayBaseURL(),
		ExtensionPromptAppend: extPrompt,
		StateDir:              cfg.StateDir,
		LocalIntel: agent.LocalIntelRunnerConfig{
			Enabled:      cfg.Agent.LocalIntel.Enabled,
			GGUFPath:     cfg.Agent.LocalIntel.GGUFPath,
			MaxTokens:    cfg.Agent.LocalIntel.MaxTokens,
			SystemPrompt: cfg.Agent.LocalIntel.SystemPrompt,
			CacheDir:     localIntelCache,
		},
		Palace: agent.PalaceConfig{
			Enabled:             cfg.Agent.Palace.Enabled,
			ShadowOnly:          cfg.Agent.Palace.ShadowOnly,
			PromptRouting:       cfg.Agent.Palace.PromptRouting,
			MemorySearchRouting: cfg.Agent.Palace.MemorySearchRouting,
			ToolRouting:         cfg.Agent.Palace.ToolRouting,
			FailOpen:            cfg.Agent.Palace.FailOpen,
			LogDecisions:        cfg.Agent.Palace.LogDecisions,
		},
	}, p, toolReg, memMgr, log, cfg.StateDir).WithCatalog(catalog).WithHardware(hw)

	// ── Security: Guardrail + Audit Log ────────────────────────────────
	guardrailMode := security.GuardrailMode(cfg.Security.Mode)
	if guardrailMode == "" {
		guardrailMode = security.ModeMonitor
	}
	var ownerOnly []string
	if cfg.Security.OwnerOnlyPaths != nil {
		ownerOnly = *cfg.Security.OwnerOnlyPaths
	}
	guardrail, guardErr := security.NewGuardrail(guardrailMode, cfg.Security.BlockPatterns, ownerOnly)
	if guardErr == nil && len(cfg.Security.UserDenyPaths) > 0 {
		guardrail = guardrail.WithUserDenyPaths(cfg.Security.UserDenyPaths)
	}
	if guardErr != nil {
		log.Warn("security guardrail init failed", zap.Error(guardErr))
	}

	secLogPath := cfg.Security.LogPath
	if secLogPath == "" {
		secLogPath = filepath.Join(cfg.StateDir, "security", "audit.ndjson")
	}
	auditLog, auditErr := security.NewAuditLog(secLogPath, cfg.Security.PIIMask, log)
	if auditErr != nil {
		log.Warn("security audit log init failed", zap.Error(auditErr))
	}

	runner = runner.WithSecurity(guardrail, auditLog)
	log.Info("security layer active",
		zap.String("mode", string(guardrailMode)),
		zap.String("audit_log", secLogPath),
	)

	// Sub-agents (delegation): register after security so child runs inherit guardrail/audit.
	subSvc := &tools.SubAgentSvc{
		Store:                 subagents.NewStore(cfg.StateDir),
		Provider:              p,
		ParentRegistry:        toolReg,
		ToolGraph:             toolGraph,
		Mem:                   memMgr,
		Log:                   log,
		Model:                 modelInfo.ID,
		ActiveSkillsContext:   skillsCtx,
		ProviderName:          providerNameForCaps,
		GatewayPublicBaseURL:  cfg.PublicGatewayBaseURL(),
		ExtensionPromptAppend: extPrompt,
		DefaultToolsProfile:   cfg.Security.Profile,
		Guardrail:             guardrail,
		AuditLog:              auditLog,
		Hardware:              hw,
	}
	toolReg.Register(tools.SubAgentCreateTool{S: subSvc})
	toolReg.Register(tools.SubAgentListTool{S: subSvc})
	toolReg.Register(tools.SubAgentRunTool{S: subSvc})
	toolReg.Register(tools.SubAgentRemoveTool{S: subSvc})
	catalog = tools.NewCatalog(toolReg, toolGraph)
	runner = runner.WithCatalog(catalog).WithModelRegistry(reg)

	// ── Awareness (live context: time of day, presence, calendar) ──────
	awareStore := awareness.NewStore(cfg.StateDir)
	awareness.StartIdlePoller(ctx, awareStore, time.Minute)
	runner = runner.WithAwareness(awareStore)

	// ── Cron Daemon ───────────────────────────────────────────────────
	var cronJobs []cron.Job
	for _, j := range cfg.Cron {
		cronJobs = append(cronJobs, cron.Job{
			ID:         j.ID,
			Schedule:   j.Schedule,
			Prompt:     j.Prompt,
			MaxRetries: j.MaxRetries,
		})
	}

	cronDaemon := cron.NewDaemon(
		cronJobs,
		runner,
		log,
		filepath.Join(cfg.StateDir, "cron_jobs.json"),
	).WithFailureNotifier(func(ctx context.Context, jobID, summary string) {
		// Default failure path: structured log + episodic memory note so
		// the user discovers it on next interactive session. Channel-side
		// notifications are layered on top by the gateway when available.
		log.Error("cron failure notification", zap.String("id", jobID), zap.String("summary", summary))
		if memMgr != nil && memMgr.Episodic != nil {
			_ = memMgr.Episodic.Save(ctx, memory.Message{
				ID:        uuid.New().String(),
				SessionID: "cron:failures",
				Role:      memory.RoleSystem,
				Content:   "[CRON FAILURE] " + summary,
				CreatedAt: time.Now(),
			})
		}
	})
	if err := cronDaemon.Start(); err != nil {
		log.Warn("failed to start cron daemon", zap.Error(err))
	} else {
		log.Info("cron daemon started", zap.Int("static_jobs", len(cronJobs)))
		app.Cron = cronDaemon
	}

	app.Runner = runner
	app.Provider = p
	app.ModelID = modelInfo.ID
	app.ChannelSenders = channelSenders
	app.Aware = awareStore
	return app, nil
}
