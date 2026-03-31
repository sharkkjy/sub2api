// Package claude provides constants and helpers for Claude API integration.
package claude

import (
	"fmt"
	"sync"
)

// Claude Code 客户端相关常量

// Beta header 常量
const (
	BetaOAuth                   = "oauth-2025-04-20"
	BetaClaudeCode              = "claude-code-20250219"
	BetaInterleavedThinking     = "interleaved-thinking-2025-05-14"
	BetaFineGrainedToolStreaming = "fine-grained-tool-streaming-2025-05-14"
	BetaTokenCounting           = "token-counting-2024-11-01"
	BetaContext1M               = "context-1m-2025-08-07"
	BetaFastMode                = "fast-mode-2026-02-01"
	BetaTokenEfficientTools     = "token-efficient-tools-2026-03-28"
	BetaContextManagement       = "context-management-2025-06-27"
	BetaRedactThinking          = "redact-thinking-2026-02-12"
	BetaPromptCachingScope      = "prompt-caching-scope-2026-01-05"
	BetaStructuredOutputs       = "structured-outputs-2025-12-15"
	BetaAdvancedToolUse         = "advanced-tool-use-2025-11-20"
	BetaEffort                  = "effort-2025-11-24"
	BetaTaskBudgets             = "task-budgets-2026-03-13"
	BetaAdvisor                 = "advisor-tool-2026-03-01"
	BetaWebSearch               = "web-search-2025-03-05"
)

// DroppedBetas 是转发时需要从 anthropic-beta header 中移除的 beta token 列表。
// 这些 token 是客户端特有的，不应透传给上游 API。
var DroppedBetas = []string{}

// DefaultBetaHeader Claude Code 客户端默认的 anthropic-beta header
const DefaultBetaHeader = BetaClaudeCode + "," + BetaOAuth + "," + BetaInterleavedThinking + "," + BetaTokenEfficientTools

// MessageBetaHeaderNoTools /v1/messages 在无工具时的 beta header
//
// NOTE: Claude Code OAuth credentials are scoped to Claude Code. When we "mimic"
// Claude Code for non-Claude-Code clients, we must include the claude-code beta
// even if the request doesn't use tools, otherwise upstream may reject the
// request as a non-Claude-Code API request.
const MessageBetaHeaderNoTools = BetaClaudeCode + "," + BetaOAuth + "," + BetaInterleavedThinking + "," + BetaTokenEfficientTools

// MessageBetaHeaderWithTools /v1/messages 在有工具时的 beta header
const MessageBetaHeaderWithTools = BetaClaudeCode + "," + BetaOAuth + "," + BetaInterleavedThinking + "," + BetaTokenEfficientTools

// CountTokensBetaHeader count_tokens 请求使用的 anthropic-beta header
const CountTokensBetaHeader = BetaClaudeCode + "," + BetaOAuth + "," + BetaInterleavedThinking + "," + BetaTokenCounting

// HaikuBetaHeader Haiku 模型使用的 anthropic-beta header（不需要 claude-code beta）
const HaikuBetaHeader = BetaOAuth + "," + BetaInterleavedThinking

// APIKeyBetaHeader API-key 账号建议使用的 anthropic-beta header（不包含 oauth）
const APIKeyBetaHeader = BetaClaudeCode + "," + BetaInterleavedThinking + "," + BetaTokenEfficientTools

// APIKeyHaikuBetaHeader Haiku 模型在 API-key 账号下使用的 anthropic-beta header（不包含 oauth / claude-code）
const APIKeyHaikuBetaHeader = BetaInterleavedThinking

// --- Dynamic Version Management ---

// versionInfo holds the current version information that can be updated at runtime.
// Protected by a RWMutex for concurrent access.
type versionInfo struct {
	mu              sync.RWMutex
	cliVersion      string // e.g., "2.1.88"
	sdkVersion      string // e.g., "0.74.0"
	nodeVersion     string // e.g., "v22.14.0"
}

// defaultVersionInfo holds the fallback version values.
// These are used when the version syncer hasn't updated yet.
var currentVersion = &versionInfo{
	cliVersion:  "2.1.88",
	sdkVersion:  "0.74.0",
	nodeVersion: "v22.14.0",
}

// GetCurrentCLIVersion returns the current Claude CLI version.
func GetCurrentCLIVersion() string {
	currentVersion.mu.RLock()
	defer currentVersion.mu.RUnlock()
	return currentVersion.cliVersion
}

// GetCurrentSDKVersion returns the current Anthropic SDK version.
func GetCurrentSDKVersion() string {
	currentVersion.mu.RLock()
	defer currentVersion.mu.RUnlock()
	return currentVersion.sdkVersion
}

// GetCurrentNodeVersion returns the current Node.js version.
func GetCurrentNodeVersion() string {
	currentVersion.mu.RLock()
	defer currentVersion.mu.RUnlock()
	return currentVersion.nodeVersion
}

// GetCurrentUserAgent returns the current User-Agent string.
func GetCurrentUserAgent() string {
	currentVersion.mu.RLock()
	defer currentVersion.mu.RUnlock()
	return fmt.Sprintf("claude-cli/%s (external, cli)", currentVersion.cliVersion)
}

// UpdateVersions updates the CLI and SDK versions atomically.
// Called by the version syncer when new versions are detected.
func UpdateVersions(cliVersion, sdkVersion, nodeVersion string) {
	currentVersion.mu.Lock()
	defer currentVersion.mu.Unlock()
	if cliVersion != "" {
		currentVersion.cliVersion = cliVersion
	}
	if sdkVersion != "" {
		currentVersion.sdkVersion = sdkVersion
	}
	if nodeVersion != "" {
		currentVersion.nodeVersion = nodeVersion
	}
}

// GetCurrentDefaultHeaders returns a fresh copy of the default headers using
// the current (possibly auto-updated) version values.
// This replaces the static DefaultHeaders map for the forwarding code path.
func GetCurrentDefaultHeaders() map[string]string {
	currentVersion.mu.RLock()
	defer currentVersion.mu.RUnlock()
	return map[string]string{
		"User-Agent":                                fmt.Sprintf("claude-cli/%s (external, cli)", currentVersion.cliVersion),
		"X-Stainless-Lang":                          "js",
		"X-Stainless-Package-Version":               currentVersion.sdkVersion,
		"X-Stainless-OS":                            "Linux",
		"X-Stainless-Arch":                          "arm64",
		"X-Stainless-Runtime":                       "node",
		"X-Stainless-Runtime-Version":               currentVersion.nodeVersion,
		"X-Stainless-Retry-Count":                   "0",
		"X-Stainless-Timeout":                       "600",
		"X-App":                                     "cli",
		"Anthropic-Dangerous-Direct-Browser-Access": "true",
	}
}

// DefaultHeaders 是 Claude Code 客户端默认请求头。
// NOTE: 这个 map 现在仅作为初始值/fallback。实际转发时应使用 GetCurrentDefaultHeaders()
// 以获取可能已通过版本同步更新的值。
var DefaultHeaders = map[string]string{
	// Keep these in sync with recent Claude CLI traffic to reduce the chance
	// that Claude Code-scoped OAuth credentials are rejected as "non-CLI" usage.
	// Synced with Claude Code 2.1.88 / @anthropic-ai/sdk 0.74.0
	"User-Agent":                                "claude-cli/2.1.88 (external, cli)",
	"X-Stainless-Lang":                          "js",
	"X-Stainless-Package-Version":               "0.74.0",
	"X-Stainless-OS":                            "Linux",
	"X-Stainless-Arch":                          "arm64",
	"X-Stainless-Runtime":                       "node",
	"X-Stainless-Runtime-Version":               "v22.14.0",
	"X-Stainless-Retry-Count":                   "0",
	"X-Stainless-Timeout":                       "600",
	"X-App":                                     "cli",
	"Anthropic-Dangerous-Direct-Browser-Access": "true",
}

// Model 表示一个 Claude 模型
type Model struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
}

// DefaultModels Claude Code 客户端支持的默认模型列表
var DefaultModels = []Model{
	{
		ID:          "claude-opus-4-5-20251101",
		Type:        "model",
		DisplayName: "Claude Opus 4.5",
		CreatedAt:   "2025-11-01T00:00:00Z",
	},
	{
		ID:          "claude-opus-4-6",
		Type:        "model",
		DisplayName: "Claude Opus 4.6",
		CreatedAt:   "2026-02-06T00:00:00Z",
	},
	{
		ID:          "claude-sonnet-4-6",
		Type:        "model",
		DisplayName: "Claude Sonnet 4.6",
		CreatedAt:   "2026-02-18T00:00:00Z",
	},
	{
		ID:          "claude-sonnet-4-5-20250929",
		Type:        "model",
		DisplayName: "Claude Sonnet 4.5",
		CreatedAt:   "2025-09-29T00:00:00Z",
	},
	{
		ID:          "claude-haiku-4-5-20251001",
		Type:        "model",
		DisplayName: "Claude Haiku 4.5",
		CreatedAt:   "2025-10-01T00:00:00Z",
	},
}

// DefaultModelIDs 返回默认模型的 ID 列表
func DefaultModelIDs() []string {
	ids := make([]string, len(DefaultModels))
	for i, m := range DefaultModels {
		ids[i] = m.ID
	}
	return ids
}

// DefaultTestModel 测试时使用的默认模型
const DefaultTestModel = "claude-sonnet-4-5-20250929"

// ModelIDOverrides Claude OAuth 请求需要的模型 ID 映射
var ModelIDOverrides = map[string]string{
	"claude-sonnet-4-5": "claude-sonnet-4-5-20250929",
	"claude-opus-4-5":   "claude-opus-4-5-20251101",
	"claude-haiku-4-5":  "claude-haiku-4-5-20251001",
}

// ModelIDReverseOverrides 用于将上游模型 ID 还原为短名
var ModelIDReverseOverrides = map[string]string{
	"claude-sonnet-4-5-20250929": "claude-sonnet-4-5",
	"claude-opus-4-5-20251101":   "claude-opus-4-5",
	"claude-haiku-4-5-20251001":  "claude-haiku-4-5",
}

// NormalizeModelID 根据 Claude OAuth 规则映射模型
func NormalizeModelID(id string) string {
	if id == "" {
		return id
	}
	if mapped, ok := ModelIDOverrides[id]; ok {
		return mapped
	}
	return id
}

// DenormalizeModelID 将上游模型 ID 转换为短名
func DenormalizeModelID(id string) string {
	if id == "" {
		return id
	}
	if mapped, ok := ModelIDReverseOverrides[id]; ok {
		return mapped
	}
	return id
}
