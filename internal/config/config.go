// Package config loads and validates AssistClaw configuration from YAML.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/assistclaw/assistclaw/internal/mempalace"
	"gopkg.in/yaml.v3"
)

// Config is the root configuration structure for AssistClaw.
type Config struct {
	// Version identifies the config schema version for migration.
	Version int `yaml:"version"`

	// StateDir is the root directory for all AssistClaw state (~/.assistclaw by default).
	StateDir string `yaml:"state_dir"`

	// Gateway configures the HTTP/WebSocket gateway.
	Gateway GatewayConfig `yaml:"gateway"`

	// Providers holds LLM provider credentials and settings.
	Providers ProvidersConfig `yaml:"providers"`

	// Embeddings configures embedding model providers.
	Embeddings EmbeddingsConfig `yaml:"embeddings"`

	// Memory configures the three-tier memory system.
	Memory MemoryConfig `yaml:"memory"`

	// Routing defines multi-model task routing rules.
	Routing RoutingConfig `yaml:"routing"`

	// Hardware configures C++ sensing integration.
	Hardware HardwareConfig `yaml:"hardware"`

	// Agent configures the agent runner behavior.
	Agent AgentConfig `yaml:"agent"`

	// Channels configures messaging channel integrations.
	Channels ChannelsConfig `yaml:"channels"`

	// Plano configures the optional smart AI routing proxy.
	Plano PlanoConfig `yaml:"plano"`

	// MCP configures the Model Context Protocol server and external MCP client connections.
	MCP MCPConfig `yaml:"mcp"`

	// Security configures the runtime safety guardrail and audit log.
	Security SecurityConfig `yaml:"security"`

	// Cron configures static scheduled jobs.
	Cron []CronJobConfig `yaml:"cron"`

	// A2A configures the Agent-to-Agent protocol support.
	A2A A2AConfig `yaml:"a2a"`

	// Webhooks configures generic incoming webhook handlers.
	Webhooks WebhookConfig `yaml:"webhooks"`

	// Gmail configures Gmail Pub/Sub watcher settings.
	Gmail GmailConfig `yaml:"gmail"`

	// Email configures the autonomous email assistant (IMAP/Gmail/Graph).
	Email EmailConfig `yaml:"email"`

	// Voice configures internal STT/TTS and continuous conversation.
	Voice VoiceConfig `yaml:"voice"`

	// Extensions configures optional hooks available in AssistClaw (prompt
	// fragments only; there is no Node plugin loader). See `assistclaw extensions list`.
	Extensions ExtensionsConfig `yaml:"extensions"`

	// Tracing configures optional OpenTelemetry tracing.
	Tracing TracingConfig `yaml:"tracing"`
}

// ExtensionsConfig holds lightweight extension points: optional markdown merged into the system prompt.
type ExtensionsConfig struct {
	Enabled bool `yaml:"enabled"`
	// PromptFiles are paths to UTF-8 text/markdown files merged into the system prompt when
	// Enabled is true. Relative paths resolve under StateDir (e.g. extensions/extra-prompt.md).
	PromptFiles []string `yaml:"prompt_files"`
}

// TracingConfig controls optional OpenTelemetry tracing.
type TracingConfig struct {
	Enabled      bool    `yaml:"enabled"`
	OTLPEndpoint string  `yaml:"otlp_endpoint"` // e.g. localhost:4317
	ServiceName  string  `yaml:"service_name"`
	SampleRatio  float64 `yaml:"sample_ratio"` // 0..1
}

// A2AConfig holds metadata for the Agent-to-Agent protocol.
type A2AConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	AgentID     string `yaml:"agent_id"` // Optional UUID or stable identifier
}

type CronJobConfig struct {
	ID       string `yaml:"id"`
	Schedule string `yaml:"schedule"`
	Prompt   string `yaml:"prompt"`
}

// WebhookConfig configures the incoming webhook endpoint.
type WebhookConfig struct {
	Enabled  bool             `yaml:"enabled"`
	Token    string           `yaml:"token"` // Optional auth token (X-AssistClaw-Token)
	Mappings []WebhookMapping `yaml:"mappings"`
}

// WebhookMapping defines how an incoming webhook maps to an agent action.
type WebhookMapping struct {
	Path           string `yaml:"path"`            // Match /api/webhook/{path}
	PromptTemplate string `yaml:"prompt_template"` // Template for agent prompt (e.g. "Got webhook from {{.source}}: {{.body}}")
	Deliver        bool   `yaml:"deliver"`         // Whether to deliver results to a channel
	Channel        string `yaml:"channel"`         // Channel title to deliver to (e.g. "telegram")
	To             string `yaml:"to"`              // Destination account
	AllowUnsafe    bool   `yaml:"allow_unsafe"`    // If true, doesn't wrap payload in safety boundaries
}

// GmailConfig holds settings for the Gmail Pub/Sub integration.
type GmailConfig struct {
	Enabled      bool   `yaml:"enabled"`
	Account      string `yaml:"account"`
	Topic        string `yaml:"topic"`
	Label        string `yaml:"label"`         // e.g. "INBOX"
	SkipWatcher  bool   `yaml:"skip_watcher"`  // If true, AssistClaw won't manage the gogcli daemon
	PushEndpoint string `yaml:"push_endpoint"` // Public URL for Pub/Sub push (if not using Tailscale)
}

// EmailConfig configures the autonomous email assistant (summaries + draft replies with approval).
type EmailConfig struct {
	Enabled bool `yaml:"enabled"`
	// Model overrides routing.default for email LLM calls when non-empty.
	Model string `yaml:"model"`
	// Notify is the default channel + session for draft notifications and approvals.
	Notify EmailNotifyConfig `yaml:"notify"`
	// MaxDraftsPerHour limits LLM-driven drafts per account (0 = default 30).
	MaxDraftsPerHour int `yaml:"max_drafts_per_hour"`
	// PollInterval is the fallback poll period for gmail/graph when push is unavailable (default 60s).
	PollInterval string `yaml:"poll_interval"`
	// Accounts lists mailboxes to watch.
	Accounts []EmailAccountConfig `yaml:"accounts"`
}

// EmailNotifyConfig selects where mail notifications are delivered.
type EmailNotifyConfig struct {
	Channel   string `yaml:"channel"`    // telegram | discord | slack
	SessionID string `yaml:"session_id"` // e.g. tg:123 — required for outbound sends
}

// EmailAccountConfig is one watched mailbox.
type EmailAccountConfig struct {
	Name    string `yaml:"name"`
	Backend string `yaml:"backend"` // imap | gmail | graph
	IMAP    *EmailIMAPConfig       `yaml:"imap"`
	SMTP    *EmailSMTPConfig       `yaml:"smtp"`
	Gmail   *EmailGmailAPIConfig  `yaml:"gmail"`
	Graph   *EmailGraphAPIConfig  `yaml:"graph"`
	// Notify overrides root email.notify for this account when set.
	Notify *EmailNotifyConfig `yaml:"notify"`
	Rules  []EmailRuleConfig  `yaml:"rules"`
}

// EmailIMAPConfig is used when backend: imap.
type EmailIMAPConfig struct {
	Host     string `yaml:"host"`      // e.g. imap.gmail.com:993
	Username string `yaml:"username"`
	Password string `yaml:"password"` // app password; prefer ${ENV}
	Mailbox  string `yaml:"mailbox"` // default INBOX
	UseTLS   *bool  `yaml:"use_tls"` // default true for :993
}

// EmailSMTPConfig is used for sending approved replies when backend: imap.
type EmailSMTPConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"` // default 587
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	StartTLS bool   `yaml:"starttls"` // default true for 587
}

// EmailGmailAPIConfig is used when backend: gmail (OAuth token file from assistclaw email login).
type EmailGmailAPIConfig struct {
	TokenFile string `yaml:"token_file"` // path under state_dir or absolute
}

// EmailGraphAPIConfig is used when backend: graph (OAuth token file).
type EmailGraphAPIConfig struct {
	TokenFile string `yaml:"token_file"`
}

// EmailRuleConfig filters which messages trigger the assistant.
type EmailRuleConfig struct {
	Match  EmailRuleMatch `yaml:"match"`
	Action string         `yaml:"action"` // auto | notify_only | ignore
}

// EmailRuleMatch is evaluated in order; first match wins.
type EmailRuleMatch struct {
	From        string `yaml:"from"`         // exact address
	FromDomain  string `yaml:"from_domain"`  // suffix match @domain
	FromRegex   string `yaml:"from_regex"`
	Subject     string `yaml:"subject"` // regex on subject
	HeaderName  string `yaml:"header_name"`
	HeaderRegex string `yaml:"header_regex"`
	GmailLabel  string `yaml:"gmail_label"` // contains this label id or name
	GraphCat    string `yaml:"graph_category"`
}

// VoiceConfig configures internal voice processing (STT/TTS).
type VoiceConfig struct {
	Enabled       bool   `yaml:"enabled"`
	ServicePort   int    `yaml:"service_port"`    // default: 11000
	STTModel      string `yaml:"stt_model"`       // whisper: tiny, base, small, medium, large
	TTSModel      string `yaml:"tts_model"`       // voxcpm
	VoiceCloneRef string `yaml:"voice_clone_ref"` // Path to 5s voice clip
	VenvPath      string `yaml:"venv_path"`       // Path to py venv
}

// MCPConfig configures the MCP server and external MCP client connections.
type MCPConfig struct {
	Server  MCPServerConfig   `yaml:"server"`
	Clients []MCPClientConfig `yaml:"clients"`
}

// MCPServerConfig configures the built-in MCP server.
type MCPServerConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Transport string `yaml:"transport"`  // "stdio" | "http" — default: stdio
	HTTPPort  int    `yaml:"http_port"`  // default: 5173
	AuthToken string `yaml:"auth_token"` // optional bearer token for HTTP mode
}

// MCPClientConfig configures a connection to an external MCP server.
type MCPClientConfig struct {
	Name      string   `yaml:"name"`
	Transport string   `yaml:"transport"` // "stdio" | "http"
	Command   string   `yaml:"command"`   // e.g. "npx @modelcontextprotocol/server-filesystem /tmp"
	Args      []string `yaml:"args"`
	Dir       string   `yaml:"dir"` // stdio: working directory for the child process
	Env       []string `yaml:"env"` // stdio: extra KEY=value entries for the child environment
	URL       string   `yaml:"url"`
	AuthToken string   `yaml:"auth_token"`
}

// PlanoConfig configures the Plano smart routing proxy.
type PlanoConfig struct {
	Enabled          bool              `yaml:"enabled"`
	Endpoint         string            `yaml:"endpoint"`          // default: http://localhost:12000/v1
	FallbackProvider string            `yaml:"fallback_provider"` // provider name: openai, groq, etc.
	Preferences      []PlanoPreference `yaml:"preferences"`
}

// PlanoPreference maps a plain-English routing description to a preferred model.
type PlanoPreference struct {
	Description string `yaml:"description"`
	PreferModel string `yaml:"prefer_model"`
}

// GatewayConfig controls the HTTP/WebSocket gateway.
type GatewayConfig struct {
	Host  string `yaml:"host"`
	Port  int    `yaml:"port"`
	Token string `yaml:"token"`
	TLS   struct {
		Cert string `yaml:"cert"`
		Key  string `yaml:"key"`
	} `yaml:"tls"`
	Bind      string          `yaml:"bind"` // loopback, lan, tailnet, custom
	Tailscale TailscaleConfig `yaml:"tailscale"`
}

type TailscaleConfig struct {
	Mode        string `yaml:"mode"` // off, serve, funnel
	ResetOnExit bool   `yaml:"reset_on_exit"`
}

// ProvidersConfig holds all LLM provider configurations.
type ProvidersConfig struct {
	OpenAI      *ProviderCreds    `yaml:"openai"`
	AzureOpenAI *AzureCreds       `yaml:"azure_openai"`
	Anthropic   *ProviderCreds    `yaml:"anthropic"`
	Bedrock     *BedrockCreds     `yaml:"bedrock"`
	Vertex      *VertexCreds      `yaml:"vertex"`
	Ollama      *LocalCreds       `yaml:"ollama"`
	VLLM        *LocalCreds       `yaml:"vllm"`
	LMStudio    *LocalCreds       `yaml:"lm_studio"`
	Groq        *ProviderCreds    `yaml:"groq"`
	Mistral     *ProviderCreds    `yaml:"mistral"`
	Together    *ProviderCreds    `yaml:"together"`
	OpenRouter  *OpenRouterCreds  `yaml:"openrouter"`
	NVIDIA      *ProviderCreds    `yaml:"nvidia"`
	Cohere      *ProviderCreds    `yaml:"cohere"`
	DeepSeek    *ProviderCreds    `yaml:"deepseek"`
	Perplexity  *ProviderCreds    `yaml:"perplexity"`
	XAI         *ProviderCreds    `yaml:"xai"`
	Voyage      *ProviderCreds    `yaml:"voyage"`
	HuggingFace *HuggingFaceCreds `yaml:"huggingface"`
}

// ProviderCreds holds API key and optional settings for a cloud provider.
type ProviderCreds struct {
	APIKey       string `yaml:"api_key"`
	BaseURL      string `yaml:"base_url"`
	DefaultModel string `yaml:"default_model"`
}

// AzureCreds adds Azure-specific fields.
type AzureCreds struct {
	ProviderCreds `yaml:",inline"`
	APIVersion    string `yaml:"api_version"`
}

// BedrockCreds holds AWS Bedrock authentication settings.
type BedrockCreds struct {
	Region          string `yaml:"region"`
	Profile         string `yaml:"profile"` // AWS named profile
	AccessKeyID     string `yaml:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key"`
	APIKey          string `yaml:"api_key"`
	DefaultModel    string `yaml:"default_model"`
}

// VertexCreds holds Google Vertex AI settings.
type VertexCreds struct {
	ProjectID    string `yaml:"project_id"`
	Location     string `yaml:"location"`
	Credentials  string `yaml:"credentials"` // path to service account JSON
	DefaultModel string `yaml:"default_model"`
}

// LocalCreds configures a local server (Ollama, vLLM, LM Studio).
type LocalCreds struct {
	BaseURL      string `yaml:"base_url"`
	APIKey       string `yaml:"api_key"` // optional for vLLM
	DefaultModel string `yaml:"default_model"`
}

// OpenRouterCreds adds OpenRouter-specific fields.
type OpenRouterCreds struct {
	ProviderCreds `yaml:",inline"`
	SiteName      string `yaml:"site_name"`
	SiteURL       string `yaml:"site_url"`
}

// HuggingFaceCreds adds HuggingFace-specific fields.
type HuggingFaceCreds struct {
	ProviderCreds `yaml:",inline"`
	Model         string `yaml:"model"` // specific model endpoint
}

// EmbeddingsConfig configures embedding provider priority.
type EmbeddingsConfig struct {
	// Priority lists providers in order of preference.
	// Accepted values: openai, cohere, google, ollama, huggingface
	Priority    []string          `yaml:"priority"`
	OpenAI      *ProviderCreds    `yaml:"openai"`
	AzureOpenAI *AzureCreds       `yaml:"azure_openai"`
	Cohere      *ProviderCreds    `yaml:"cohere"`
	Google      *ProviderCreds    `yaml:"google"`
	OllamaEmbed *LocalCreds       `yaml:"ollama"`
	Bedrock     *BedrockCreds     `yaml:"bedrock"`
	Voyage      *ProviderCreds    `yaml:"voyage"`
	Mistral     *ProviderCreds    `yaml:"mistral"`
	Vertex      *VertexCreds      `yaml:"vertex"`
	HuggingFace *HuggingFaceCreds `yaml:"huggingface"`
}

// MemoryConfig controls the memory system.
type MemoryConfig struct {
	// WorkingTokenBudget is the max token count kept in working memory.
	WorkingTokenBudget int `yaml:"working_token_budget"`
	// EpisodicDBPath is the path to the episodic SQLite database.
	EpisodicDBPath string `yaml:"episodic_db_path"`
	// SemanticDBPath is the path to the semantic sqlite-vec database.
	SemanticDBPath string `yaml:"semantic_db_path"`
	// SemanticBackend selects AssistClaw's built-in semantic store (sqlite-vec only today).
	SemanticBackend string `yaml:"semantic_backend"`
	// MemPalace wires the upstream MemPalace project (Python + MCP). See memory.mempalace and mcp.clients.
	MemPalace MemoryMemPalaceConfig `yaml:"mempalace"`
	// Mining configures optional taxonomy mining/backfill runs.
	Mining MemoryMiningConfig `yaml:"mining"`
}

// MemoryMemPalaceConfig integrates the real MemPalace memory system via MCP (python -m mempalace.mcp_server).
// It does not replace AssistClaw's episodic DB; it adds MemPalace tools and optional delegation from memory_search.
type MemoryMemPalaceConfig struct {
	Enabled bool `yaml:"enabled"`
	// MCPClientName must match an entry in mcp.clients[].name or the synthetic client when AutoStart (default: mempalace).
	MCPClientName string `yaml:"mcp_client_name"`
	// AutoStart runs MemPalace as a stdio MCP child process (in-process sidecar) when no mcp.clients
	// entry exists for MCPClientName. Recommended for single-binary installs; use explicit mcp.clients
	// or an external supervisor instead when you want a true out-of-process sidecar.
	AutoStart bool `yaml:"auto_start"`
	// ManagedVenv creates state_dir/mempalace/venv, pip-installs mempalace, runs mempalace init once,
	// and pins PythonExecutable to that venv. Requires auto_start: true. See `assistclaw mempalace setup`.
	ManagedVenv bool `yaml:"managed_venv"`
	// BootstrapPython is the host interpreter used only to create the managed venv (stdlib venv module).
	// Default: python3, or ASSISTCLAW_MEMPALACE_BOOTSTRAP_PYTHON.
	BootstrapPython string `yaml:"bootstrap_python"`
	// PythonExecutable is the interpreter for `python -m mempalace.mcp_server` when AutoStart is true.
	// Override with env ASSISTCLAW_MEMPALACE_PYTHON if unset in YAML.
	PythonExecutable string `yaml:"python_executable"`
	// InjectIntoMemorySearch appends MemPalace MCP mempalace_search results to the built-in memory_search tool output.
	InjectIntoMemorySearch bool `yaml:"inject_into_memory_search"`
	// SearchLimit caps the limit argument passed to mempalace_search (0 = use the memory_search limit).
	SearchLimit int `yaml:"search_limit"`
}

type MemoryMiningConfig struct {
	Enabled        bool     `yaml:"enabled"`
	Mode           string   `yaml:"mode"` // incremental | full
	Include        []string `yaml:"include"`
	Exclude        []string `yaml:"exclude"`
	ChunkSize      int      `yaml:"chunk_size"`
	ChunkOverlap   int      `yaml:"chunk_overlap"`
	MaxFileSizeKB  int      `yaml:"max_file_size_kb"`
	MaxFilesPerRun int      `yaml:"max_files_per_run"`
	StatePath      string   `yaml:"state_path"`
}

// RoutingConfig defines multi-model routing rules.
type RoutingConfig struct {
	// Default model used when no rule matches.
	Default string `yaml:"default"`
	// Fallback model used when the primary provider is unavailable.
	Fallback string `yaml:"fallback"`
	// Rules maps task types to model strings (e.g. "ollama/llama3.2").
	Rules []RoutingRule `yaml:"rules"`
}

// RoutingRule maps a task type to a specific model.
type RoutingRule struct {
	Task  string `yaml:"task"`
	Model string `yaml:"model"`
}

// HardwareConfig controls C++ sensing integration.
type HardwareConfig struct {
	Camera CameraConfig `yaml:"camera"`
	Audio  AudioConfig  `yaml:"audio"`
	GPIO   GPIOConfig   `yaml:"gpio"`
}

// CameraConfig configures the camera sensing process.
type CameraConfig struct {
	Enabled     bool   `yaml:"enabled"`
	BinaryPath  string `yaml:"binary_path"`
	DeviceIndex int    `yaml:"device_index"`
	Width       int    `yaml:"width"`
	Height      int    `yaml:"height"`
	FPS         int    `yaml:"fps"`
}

// AudioConfig configures the audio sensing process.
type AudioConfig struct {
	Enabled    bool   `yaml:"enabled"`
	BinaryPath string `yaml:"binary_path"`
	SampleRate int    `yaml:"sample_rate"`
	Channels   int    `yaml:"channels"`
}

// GPIOConfig configures GPIO control.
type GPIOConfig struct {
	Enabled    bool   `yaml:"enabled"`
	BinaryPath string `yaml:"binary_path"`
}

// AgentConfig controls agent runner behavior.
type AgentConfig struct {
	MaxIterations   int      `yaml:"max_iterations"`
	SystemPromptExt string   `yaml:"system_prompt_ext"`
	ToolsDir        string   `yaml:"tools_dir"`
	SkillsDir       string   `yaml:"skills_dir"`
	EnabledSkills   []string `yaml:"enabled_skills"`
	// Heartbeat schedules periodic synthetic prompts (proactive ticks on a dedicated session).
	Heartbeat HeartbeatConfig `yaml:"heartbeat"`
	// Planning adds an upfront milestone breakdown (extra LLM call). Nil = enabled (default on).
	Planning *bool `yaml:"planning"`
	// Reflection adds a self-critique pass when a turn completes without tools. Nil = disabled (saves tokens).
	Reflection *bool `yaml:"reflection"`
	// Palace is optional AssistClaw-local retrieval shaping (not the MemPalace product).
	Palace PalaceConfig `yaml:"palace"`
	// LocalIntel runs Gemma 4 E2B inside the process (when built with assistclaw_localgemma) and
	// prepends a short advisory into the cloud model's system prompt for that turn.
	LocalIntel LocalIntelConfig `yaml:"local_intel"`
}

// LocalIntelConfig enables optional on-device Gemma before the main (cloud) model sees the request.
type LocalIntelConfig struct {
	Enabled bool `yaml:"enabled"`
	// GGUFPath overrides ASSISTCLAW_LOCAL_GEMMA_GGUF when non-empty.
	GGUFPath  string `yaml:"gguf_path"`
	MaxTokens int    `yaml:"max_tokens"`
	// SystemPrompt overrides the default advisory instructions when non-empty.
	SystemPrompt string `yaml:"system_prompt"`
	// CacheDir is where embedded GGUF bytes are materialized; default <state_dir>/localintel.
	CacheDir string `yaml:"cache_dir"`
}

type PalaceConfig struct {
	Enabled             bool `yaml:"enabled"`
	ShadowOnly          bool `yaml:"shadow_only"`
	PromptRouting       bool `yaml:"prompt_routing"`
	MemorySearchRouting bool `yaml:"memory_search_routing"`
	ToolRouting         bool `yaml:"tool_routing"`
	FailOpen            bool `yaml:"fail_open"`
	LogDecisions        bool `yaml:"log_decisions"`
}

// HeartbeatConfig drives autonomous periodic agent turns on a dedicated session.
type HeartbeatConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Interval  string `yaml:"interval"`   // e.g. 30m, 1h (default 30m when enabled)
	SessionID string `yaml:"session_id"` // dedicated session; default assistclaw:heartbeat
	Prompt    string `yaml:"prompt"`     // defaults to standard HEARTBEAT.md instruction
}

// SecurityConfig configures the runtime safety guardrail and tamper-evident audit log.
type SecurityConfig struct {
	// Mode controls how findings are acted upon.
	// Values: "monitor" (log only, default), "enforce" (block HIGH), "strict" (block MEDIUM+HIGH)
	Mode string `yaml:"mode"`

	// LogPath is the path to the NDJSON audit log.
	// Defaults to <state_dir>/security/audit.ndjson
	LogPath string `yaml:"log_path"`

	// PIIMask replaces detected PII in audit log entries with [REDACTED:<type>].
	PIIMask bool `yaml:"pii_mask"`

	// BlockPatterns are additional regex patterns to block in input.
	BlockPatterns []string `yaml:"block_patterns"`

	// Profile determines which tools are available. "full" or "coding"
	Profile string `yaml:"profile"`

	// OwnerOnlyPaths lists paths relative to state_dir that the agent must never modify
	// (write_file, edit, apply_patch, env write_file, or bash referencing those paths).
	// YAML key omitted → defaults to POLICIES.md, RULES.md, and the policies/ directory.
	// Set explicitly to an empty list (owner_only_paths: []) to disable.
	OwnerOnlyPaths *[]string `yaml:"owner_only_paths"`
}

// ChannelsConfig configures messaging channels.
type ChannelsConfig struct {
	Outbound OutboundReliabilityConfig `yaml:"outbound"`
	Telegram *TelegramConfig           `yaml:"telegram"`
	Discord  *DiscordConfig            `yaml:"discord"`
	Slack    *SlackConfig              `yaml:"slack"`
	WhatsApp *WhatsAppConfig           `yaml:"whatsapp"`
}

// OutboundReliabilityConfig configures shared retry/circuit-breaker/DLQ behavior
// for adapter outbound sends.
type OutboundReliabilityConfig struct {
	MaxAttempts      int     `yaml:"max_attempts"`
	BaseDelayMS      int     `yaml:"base_delay_ms"`
	MaxDelayMS       int     `yaml:"max_delay_ms"`
	JitterPercent    float64 `yaml:"jitter_percent"`
	BreakerThreshold int     `yaml:"breaker_threshold"`
	BreakerCooldownS int     `yaml:"breaker_cooldown_s"`
	DLQPath          string  `yaml:"dlq_path"`
}

type WhatsAppConfig struct {
	SessionID string   `yaml:"session_id"`
	DMMode    string   `yaml:"dm_mode"`    // open, pairing, allowlist, disabled
	AllowFrom []string `yaml:"allow_from"` // Whitelisted numbers
}

type TelegramConfig struct {
	BotToken       string   `yaml:"bot_token"`
	DMMode         string   `yaml:"dm_mode"`         // open, pairing, allowlist, disabled
	AllowFrom      []string `yaml:"allow_from"`      // Whitelisted IDs/Usernames
	RequireMention *bool    `yaml:"require_mention"` // Group chats require @bot mention when true (default)
}

type DiscordConfig struct {
	BotToken       string   `yaml:"bot_token"`
	DMMode         string   `yaml:"dm_mode"`         // open, pairing, allowlist, disabled
	AllowFrom      []string `yaml:"allow_from"`      // Whitelisted IDs/Usernames
	RequireMention *bool    `yaml:"require_mention"` // Guild channels require bot mention when true (default)
}

type SlackConfig struct {
	BotToken  string   `yaml:"bot_token"`
	AppToken  string   `yaml:"app_token"`
	DMMode    string   `yaml:"dm_mode"`    // open, pairing, allowlist, disabled
	AllowFrom []string `yaml:"allow_from"` // Whitelisted IDs/Usernames
}

// ─────────────────────────────────────────────
// Loading
// ─────────────────────────────────────────────

// Load reads configuration from the given file path, expanding environment
// variables in values. Returns a Config with defaults applied.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	// Expand ${ENV_VAR} patterns in the config file.
	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	applyDefaults(&cfg)
	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("config: validate: %w", err)
	}
	return &cfg, nil
}

// LoadFromEnv builds a minimal Config from environment variables only.
// Useful for containerized deployments without a config file.
func LoadFromEnv() *Config {
	cfg := &Config{}
	applyDefaults(cfg)
	applyProviderEnv(cfg)
	return cfg
}

// MemPalaceBootstrapConfig returns a config with StateDir set and defaults applied, plus the same
// provider environment wiring as [LoadFromEnv]. Use from installers / `assistclaw mempalace setup --state-dir`
// when assistclaw.yaml does not exist yet.
func MemPalaceBootstrapConfig(stateDir string) *Config {
	cfg := &Config{}
	cfg.StateDir = filepath.Clean(stateDir)
	applyDefaults(cfg)
	applyProviderEnv(cfg)
	return cfg
}

func applyProviderEnv(cfg *Config) {
	if key := os.Getenv("ASSISTCLAW_OPENAI_API_KEY"); key != "" {
		cfg.Providers.OpenAI = &ProviderCreds{APIKey: key}
	}
	if key := os.Getenv("OPENAI_API_KEY"); key != "" && cfg.Providers.OpenAI == nil {
		cfg.Providers.OpenAI = &ProviderCreds{APIKey: key}
	}
	if key := os.Getenv("ASSISTCLAW_ANTHROPIC_API_KEY"); key != "" {
		cfg.Providers.Anthropic = &ProviderCreds{APIKey: key}
	}
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" && cfg.Providers.Anthropic == nil {
		cfg.Providers.Anthropic = &ProviderCreds{APIKey: key}
	}
	if key := os.Getenv("ASSISTCLAW_GROQ_API_KEY"); key != "" {
		cfg.Providers.Groq = &ProviderCreds{APIKey: key}
	}
	if key := os.Getenv("GROQ_API_KEY"); key != "" && cfg.Providers.Groq == nil {
		cfg.Providers.Groq = &ProviderCreds{APIKey: key}
	}

	if key := os.Getenv("ASSISTCLAW_XAI_API_KEY"); key != "" {
		cfg.Providers.XAI = &ProviderCreds{APIKey: key}
	}
	if key := os.Getenv("XAI_API_KEY"); key != "" && cfg.Providers.XAI == nil {
		cfg.Providers.XAI = &ProviderCreds{APIKey: key}
	}
	if key := os.Getenv("ASSISTCLAW_MISTRAL_API_KEY"); key != "" {
		cfg.Providers.Mistral = &ProviderCreds{APIKey: key}
	}
	if key := os.Getenv("MISTRAL_API_KEY"); key != "" && cfg.Providers.Mistral == nil {
		cfg.Providers.Mistral = &ProviderCreds{APIKey: key}
	}
	if key := os.Getenv("ASSISTCLAW_OPENROUTER_API_KEY"); key != "" {
		cfg.Providers.OpenRouter = &OpenRouterCreds{ProviderCreds: ProviderCreds{APIKey: key}}
	}
	if key := os.Getenv("OPENROUTER_API_KEY"); key != "" && cfg.Providers.OpenRouter == nil {
		cfg.Providers.OpenRouter = &OpenRouterCreds{ProviderCreds: ProviderCreds{APIKey: key}}
	}
	if url := os.Getenv("ASSISTCLAW_OLLAMA_BASE_URL"); url != "" {
		cfg.Providers.Ollama = &LocalCreds{BaseURL: url}
	} else if os.Getenv("ASSISTCLAW_OLLAMA_ENABLED") == "1" || os.Getenv("ASSISTCLAW_OLLAMA_ENABLED") == "true" {
		cfg.Providers.Ollama = &LocalCreds{BaseURL: "http://localhost:11434"}
	}

	if os.Getenv("ASSISTCLAW_VOICE_ENABLED") == "1" || os.Getenv("ASSISTCLAW_VOICE_ENABLED") == "true" {
		cfg.Voice.Enabled = true
	}
}

// applyDefaults fills in default values for missing configuration.
func applyDefaults(cfg *Config) {
	if cfg.StateDir == "" {
		if env := os.Getenv("ASSISTCLAW_STATE_DIR"); env != "" {
			cfg.StateDir = env
		} else {
			home, _ := os.UserHomeDir()
			cfg.StateDir = filepath.Join(home, ".assistclaw")
		}
	}
	if cfg.Gateway.Host == "" {
		cfg.Gateway.Host = "127.0.0.1"
	}
	if cfg.Gateway.Port == 0 {
		cfg.Gateway.Port = 18790
	}
	if cfg.Memory.WorkingTokenBudget == 0 {
		cfg.Memory.WorkingTokenBudget = 100_000
	}
	if cfg.Memory.EpisodicDBPath == "" {
		cfg.Memory.EpisodicDBPath = filepath.Join(cfg.StateDir, "memory", "episodic.db")
	}
	if cfg.Memory.SemanticDBPath == "" {
		cfg.Memory.SemanticDBPath = filepath.Join(cfg.StateDir, "memory", "semantic.db")
	}
	if cfg.Memory.SemanticBackend == "" {
		cfg.Memory.SemanticBackend = "sqlite_vec"
	}
	if cfg.Memory.Mining.Mode == "" {
		cfg.Memory.Mining.Mode = "incremental"
	}
	if len(cfg.Memory.Mining.Include) == 0 {
		cfg.Memory.Mining.Include = []string{"MEMORY.md", "memory/*.md"}
	}
	if cfg.Memory.Mining.ChunkSize == 0 {
		cfg.Memory.Mining.ChunkSize = 512
	}
	if cfg.Memory.Mining.ChunkOverlap == 0 {
		cfg.Memory.Mining.ChunkOverlap = 64
	}
	if cfg.Memory.Mining.MaxFileSizeKB == 0 {
		cfg.Memory.Mining.MaxFileSizeKB = 512
	}
	if cfg.Memory.Mining.MaxFilesPerRun == 0 {
		cfg.Memory.Mining.MaxFilesPerRun = 1000
	}
	if strings.TrimSpace(cfg.Memory.Mining.StatePath) == "" {
		cfg.Memory.Mining.StatePath = filepath.Join(cfg.StateDir, "memory", "mining_state.json")
	}
	if strings.TrimSpace(cfg.Memory.MemPalace.MCPClientName) == "" {
		cfg.Memory.MemPalace.MCPClientName = "mempalace"
	}
	if cfg.Memory.MemPalace.ManagedVenv {
		if strings.TrimSpace(cfg.Memory.MemPalace.BootstrapPython) == "" {
			if env := strings.TrimSpace(os.Getenv("ASSISTCLAW_MEMPALACE_BOOTSTRAP_PYTHON")); env != "" {
				cfg.Memory.MemPalace.BootstrapPython = env
			}
		}
		if strings.TrimSpace(cfg.Memory.MemPalace.BootstrapPython) == "" {
			cfg.Memory.MemPalace.BootstrapPython = "python3"
		}
	}
	if cfg.Memory.MemPalace.ManagedVenv && cfg.Memory.MemPalace.AutoStart {
		cfg.Memory.MemPalace.PythonExecutable = mempalace.VenvInterpreter(mempalace.ManagedVenvRoot(cfg.StateDir))
	} else if cfg.Memory.MemPalace.AutoStart {
		if strings.TrimSpace(cfg.Memory.MemPalace.PythonExecutable) == "" {
			if env := strings.TrimSpace(os.Getenv("ASSISTCLAW_MEMPALACE_PYTHON")); env != "" {
				cfg.Memory.MemPalace.PythonExecutable = env
			}
		}
		if strings.TrimSpace(cfg.Memory.MemPalace.PythonExecutable) == "" {
			cfg.Memory.MemPalace.PythonExecutable = "python3"
		}
	}
	if cfg.Agent.MaxIterations == 0 {
		cfg.Agent.MaxIterations = 64
	}
	if cfg.Agent.ToolsDir == "" {
		cfg.Agent.ToolsDir = filepath.Join(cfg.StateDir, "tools")
	}
	if cfg.Agent.SkillsDir == "" {
		cfg.Agent.SkillsDir = filepath.Join(cfg.StateDir, "skills")
	}
	if strings.TrimSpace(cfg.Agent.LocalIntel.CacheDir) == "" {
		cfg.Agent.LocalIntel.CacheDir = filepath.Join(cfg.StateDir, "localintel")
	}
	if len(cfg.Embeddings.Priority) == 0 {
		cfg.Embeddings.Priority = []string{"openai", "ollama", "cohere", "voyage", "mistral", "google", "huggingface"}
	}

	// Voice Defaults
	cfg.Voice.Enabled = true // Enabled by default as an inbuilt feature
	if cfg.Voice.ServicePort == 0 {
		cfg.Voice.ServicePort = 11000
	}
	if cfg.Voice.STTModel == "" {
		cfg.Voice.STTModel = "base"
	}
	if cfg.Voice.TTSModel == "" {
		cfg.Voice.TTSModel = "voxcpm"
	}
	if cfg.Voice.VenvPath == "" {
		cfg.Voice.VenvPath = filepath.Join(cfg.StateDir, "voice_env")
	}

	if cfg.Security.OwnerOnlyPaths == nil {
		p := []string{"POLICIES.md", "RULES.md", "policies"}
		cfg.Security.OwnerOnlyPaths = &p
	}

	// Shared outbound reliability defaults (adapter send path).
	if cfg.Channels.Outbound.MaxAttempts == 0 {
		cfg.Channels.Outbound.MaxAttempts = 5
	}
	if cfg.Channels.Outbound.BaseDelayMS == 0 {
		cfg.Channels.Outbound.BaseDelayMS = 250
	}
	if cfg.Channels.Outbound.MaxDelayMS == 0 {
		cfg.Channels.Outbound.MaxDelayMS = 10_000
	}
	if cfg.Channels.Outbound.JitterPercent == 0 {
		cfg.Channels.Outbound.JitterPercent = 0.2
	}
	if cfg.Channels.Outbound.BreakerThreshold == 0 {
		cfg.Channels.Outbound.BreakerThreshold = 5
	}
	if cfg.Channels.Outbound.BreakerCooldownS == 0 {
		cfg.Channels.Outbound.BreakerCooldownS = 30
	}
	if strings.TrimSpace(cfg.Channels.Outbound.DLQPath) == "" {
		cfg.Channels.Outbound.DLQPath = filepath.Join(cfg.StateDir, "channels", "dlq.ndjson")
	}
	if strings.TrimSpace(cfg.Tracing.OTLPEndpoint) == "" {
		cfg.Tracing.OTLPEndpoint = "localhost:4317"
	}
	if strings.TrimSpace(cfg.Tracing.ServiceName) == "" {
		cfg.Tracing.ServiceName = "assistclaw"
	}
	if cfg.Tracing.SampleRatio <= 0 {
		cfg.Tracing.SampleRatio = 0.01
	}
	if !cfg.Agent.Palace.Enabled &&
		!cfg.Agent.Palace.ShadowOnly &&
		!cfg.Agent.Palace.PromptRouting &&
		!cfg.Agent.Palace.MemorySearchRouting &&
		!cfg.Agent.Palace.ToolRouting &&
		!cfg.Agent.Palace.LogDecisions &&
		!cfg.Agent.Palace.FailOpen {
		// Safe default for fresh configs while preserving explicit false in active setups.
		cfg.Agent.Palace.FailOpen = true
	}
	if cfg.Email.Enabled && cfg.Email.MaxDraftsPerHour == 0 {
		cfg.Email.MaxDraftsPerHour = 30
	}
	if cfg.Email.Enabled && strings.TrimSpace(cfg.Email.PollInterval) == "" {
		cfg.Email.PollInterval = "60s"
	}
}

// validate checks that required fields are present.
func validate(cfg *Config) error {
	var issues []string

	if cfg.Routing.Default == "" && cfg.Providers.OpenAI == nil && cfg.Providers.Anthropic == nil &&
		cfg.Providers.Ollama == nil && cfg.Providers.VLLM == nil {
		issues = append(issues, "at least one LLM provider must be configured (providers.openai, providers.anthropic, providers.ollama, etc.)")
	}

	if cfg.Gateway.Port < 1 || cfg.Gateway.Port > 65535 {
		issues = append(issues, "gateway.port must be between 1 and 65535")
	}
	if cfg.Channels.Outbound.MaxAttempts < 1 {
		issues = append(issues, "channels.outbound.max_attempts must be >= 1")
	}
	if cfg.Channels.Outbound.BaseDelayMS < 1 {
		issues = append(issues, "channels.outbound.base_delay_ms must be >= 1")
	}
	if cfg.Channels.Outbound.MaxDelayMS < cfg.Channels.Outbound.BaseDelayMS {
		issues = append(issues, "channels.outbound.max_delay_ms must be >= channels.outbound.base_delay_ms")
	}
	if cfg.Channels.Outbound.JitterPercent < 0 || cfg.Channels.Outbound.JitterPercent > 1 {
		issues = append(issues, "channels.outbound.jitter_percent must be between 0 and 1")
	}
	if cfg.Channels.Outbound.BreakerThreshold < 1 {
		issues = append(issues, "channels.outbound.breaker_threshold must be >= 1")
	}
	if cfg.Channels.Outbound.BreakerCooldownS < 1 {
		issues = append(issues, "channels.outbound.breaker_cooldown_s must be >= 1")
	}
	if cfg.Memory.SemanticBackend != "sqlite_vec" {
		if strings.EqualFold(cfg.Memory.SemanticBackend, "mempalace") {
			issues = append(issues, `memory.semantic_backend "mempalace" is invalid — MemPalace is the separate Python project; integrate it via mcp.clients or memory.mempalace (auto_start / managed_venv). Use semantic_backend: sqlite_vec.`)
		} else {
			issues = append(issues, "memory.semantic_backend must be sqlite_vec")
		}
	}
	if cfg.Memory.MemPalace.InjectIntoMemorySearch && !cfg.Memory.MemPalace.Enabled {
		issues = append(issues, "memory.mempalace.inject_into_memory_search requires memory.mempalace.enabled: true")
	}
	if cfg.Memory.MemPalace.ManagedVenv && !cfg.Memory.MemPalace.AutoStart {
		issues = append(issues, "memory.mempalace.managed_venv requires memory.mempalace.auto_start: true")
	}
	if cfg.Memory.MemPalace.Enabled {
		found := false
		for _, c := range cfg.MCP.Clients {
			if c.Name == cfg.Memory.MemPalace.MCPClientName {
				found = true
				break
			}
		}
		if !found && !cfg.Memory.MemPalace.AutoStart {
			issues = append(issues, fmt.Sprintf("memory.mempalace.enabled requires either mcp.clients with name %q or memory.mempalace.auto_start: true", cfg.Memory.MemPalace.MCPClientName))
		}
	}
	if cfg.Memory.MemPalace.SearchLimit < 0 {
		issues = append(issues, "memory.mempalace.search_limit must be >= 0")
	}
	if cfg.Memory.Mining.Mode != "incremental" && cfg.Memory.Mining.Mode != "full" {
		issues = append(issues, "memory.mining.mode must be one of: incremental, full")
	}
	if cfg.Memory.Mining.ChunkSize < 64 {
		issues = append(issues, "memory.mining.chunk_size must be >= 64")
	}
	if cfg.Memory.Mining.ChunkOverlap < 0 {
		issues = append(issues, "memory.mining.chunk_overlap must be >= 0")
	}
	if cfg.Memory.Mining.MaxFilesPerRun < 1 {
		issues = append(issues, "memory.mining.max_files_per_run must be >= 1")
	}
	if cfg.Tracing.SampleRatio < 0 || cfg.Tracing.SampleRatio > 1 {
		issues = append(issues, "tracing.sample_ratio must be between 0 and 1")
	}

	if cfg.Email.Enabled {
		if strings.TrimSpace(cfg.Email.Notify.Channel) == "" {
			issues = append(issues, "email.enabled requires email.notify.channel (telegram, discord, or slack)")
		}
		if strings.TrimSpace(cfg.Email.Notify.SessionID) == "" {
			issues = append(issues, "email.enabled requires email.notify.session_id for outbound notifications")
		}
		if len(cfg.Email.Accounts) == 0 {
			issues = append(issues, "email.enabled requires at least one email.accounts entry")
		}
		for i, a := range cfg.Email.Accounts {
			if strings.TrimSpace(a.Name) == "" {
				issues = append(issues, fmt.Sprintf("email.accounts[%d].name is required", i))
			}
			b := strings.ToLower(strings.TrimSpace(a.Backend))
			switch b {
			case "imap":
				if a.IMAP == nil || strings.TrimSpace(a.IMAP.Host) == "" {
					issues = append(issues, fmt.Sprintf("email.accounts[%q]: imap backend requires email.accounts[].imap.host", a.Name))
				}
				if a.SMTP == nil || strings.TrimSpace(a.SMTP.Host) == "" {
					issues = append(issues, fmt.Sprintf("email.accounts[%q]: imap backend requires email.accounts[].smtp.host for approved replies", a.Name))
				}
			case "gmail":
				if a.Gmail == nil || strings.TrimSpace(a.Gmail.TokenFile) == "" {
					issues = append(issues, fmt.Sprintf("email.accounts[%q]: gmail backend requires email.accounts[].gmail.token_file (run: assistclaw email login)", a.Name))
				}
			case "graph":
				if a.Graph == nil || strings.TrimSpace(a.Graph.TokenFile) == "" {
					issues = append(issues, fmt.Sprintf("email.accounts[%q]: graph backend requires email.accounts[].graph.token_file (run: assistclaw email login)", a.Name))
				}
			default:
				issues = append(issues, fmt.Sprintf("email.accounts[%q]: backend must be imap, gmail, or graph", a.Name))
			}
			for j, r := range a.Rules {
				act := strings.ToLower(strings.TrimSpace(r.Action))
				if act != "auto" && act != "notify_only" && act != "ignore" {
					issues = append(issues, fmt.Sprintf("email.accounts[%q].rules[%d].action must be auto, notify_only, or ignore", a.Name, j))
				}
			}
		}
		if cfg.Email.MaxDraftsPerHour < 1 {
			issues = append(issues, "email.max_drafts_per_hour must be >= 1 when email is enabled")
		}
		if _, err := time.ParseDuration(cfg.Email.PollInterval); err != nil {
			issues = append(issues, fmt.Sprintf("email.poll_interval: %v", err))
		}
	}

	if len(issues) > 0 {
		return fmt.Errorf("config validation errors:\n  - %s", strings.Join(issues, "\n  - "))
	}
	return nil
}

// DefaultConfigPath returns the default config file path (~/.assistclaw/assistclaw.yaml).
func DefaultConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".assistclaw", "assistclaw.yaml")
}

// PublicGatewayBaseURL returns the origin (no trailing slash) for links the agent can give users
// to open local HTML dashboards served by the gateway under /workspace/.
func (c *Config) PublicGatewayBaseURL() string {
	if c == nil {
		return ""
	}
	h := c.Gateway.Host
	if h == "" {
		h = "127.0.0.1"
	}
	p := c.Gateway.Port
	if p == 0 {
		p = 18790
	}
	return fmt.Sprintf("http://%s:%d", h, p)
}
