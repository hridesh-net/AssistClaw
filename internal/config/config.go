// Package config loads and validates AssistClaw configuration from YAML.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	// Voice configures internal STT/TTS and continuous conversation.
	Voice VoiceConfig `yaml:"voice"`

	// Extensions configures optional hooks available in AssistClaw (prompt
	// fragments only; there is no Node plugin loader). See `assistclaw extensions list`.
	Extensions ExtensionsConfig `yaml:"extensions"`
}

// ExtensionsConfig holds lightweight extension points: optional markdown merged into the system prompt.
type ExtensionsConfig struct {
	Enabled bool `yaml:"enabled"`
	// PromptFiles are paths to UTF-8 text/markdown files merged into the system prompt when
	// Enabled is true. Relative paths resolve under StateDir (e.g. extensions/extra-prompt.md).
	PromptFiles []string `yaml:"prompt_files"`
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
	Telegram *TelegramConfig `yaml:"telegram"`
	Discord  *DiscordConfig  `yaml:"discord"`
	Slack    *SlackConfig    `yaml:"slack"`
	WhatsApp *WhatsAppConfig `yaml:"whatsapp"`
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
	BotToken  string   `yaml:"bot_token"`
	DMMode    string   `yaml:"dm_mode"`    // open, pairing, allowlist, disabled
	AllowFrom []string `yaml:"allow_from"` // Whitelisted IDs/Usernames
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

	// Provider keys from environment
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
	// Ollama (local, no key needed)
	if url := os.Getenv("ASSISTCLAW_OLLAMA_BASE_URL"); url != "" {
		cfg.Providers.Ollama = &LocalCreds{BaseURL: url}
	} else if os.Getenv("ASSISTCLAW_OLLAMA_ENABLED") == "1" || os.Getenv("ASSISTCLAW_OLLAMA_ENABLED") == "true" {
		cfg.Providers.Ollama = &LocalCreds{BaseURL: "http://localhost:11434"}
	}

	// Voice
	if os.Getenv("ASSISTCLAW_VOICE_ENABLED") == "1" || os.Getenv("ASSISTCLAW_VOICE_ENABLED") == "true" {
		cfg.Voice.Enabled = true
	}

	return cfg
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
	if cfg.Agent.MaxIterations == 0 {
		cfg.Agent.MaxIterations = 64
	}
	if cfg.Agent.ToolsDir == "" {
		cfg.Agent.ToolsDir = filepath.Join(cfg.StateDir, "tools")
	}
	if cfg.Agent.SkillsDir == "" {
		cfg.Agent.SkillsDir = filepath.Join(cfg.StateDir, "skills")
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
