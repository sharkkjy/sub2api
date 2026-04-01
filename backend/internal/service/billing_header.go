package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/tidwall/gjson"
)

// billingFingerprintSalt is the hardcoded salt used by Claude Code for fingerprint computation.
// Must match the value in claude-code src/utils/fingerprint.ts: FINGERPRINT_SALT
const billingFingerprintSalt = "59cf53e54c78"

// computeBillingFingerprint computes the 3-character hex fingerprint used in the
// x-anthropic-billing-header attribution block.
//
// Algorithm: SHA256(SALT + msg[4] + msg[7] + msg[20] + version)[:3]
// where msg[i] falls back to "0" when the index is out of range.
//
// This matches the implementation in claude-code src/utils/fingerprint.ts:computeFingerprint
func computeBillingFingerprint(firstMessageText, version string) string {
	indices := []int{4, 7, 20}
	runes := []rune(firstMessageText)
	var chars strings.Builder
	for _, i := range indices {
		if i < len(runes) {
			// Use rune indexing to match JavaScript's string[i] behavior (UTF-16 code unit).
			// For BMP characters (all common text), rune index == UTF-16 code unit index.
			chars.WriteRune(runes[i])
		} else {
			chars.WriteByte('0')
		}
	}

	input := billingFingerprintSalt + chars.String() + version
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:])[:3]
}

// extractFirstUserMessageText extracts the text content from the first user message
// in the request body. Supports both string and array content formats.
func extractFirstUserMessageText(body []byte) string {
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return ""
	}

	for _, msg := range messages.Array() {
		if msg.Get("role").String() != "user" {
			continue
		}
		content := msg.Get("content")
		// String content
		if content.Type == gjson.String {
			return content.String()
		}
		// Array content: find first text block
		if content.IsArray() {
			for _, block := range content.Array() {
				if block.Get("type").String() == "text" {
					return block.Get("text").String()
				}
			}
		}
		break // Only need the first user message
	}
	return ""
}

// buildBillingHeaderText builds the x-anthropic-billing-header string that Claude Code
// injects as the first system prompt block.
//
// Format: x-anthropic-billing-header: cc_version={version}.{fingerprint}; cc_entrypoint={entrypoint};
func buildBillingHeaderText(version, fingerprint, entrypoint string) string {
	return fmt.Sprintf("x-anthropic-billing-header: cc_version=%s.%s; cc_entrypoint=%s;", version, fingerprint, entrypoint)
}

// injectBillingHeader injects the x-anthropic-billing-header block as the first system
// prompt entry in the request body. This matches what real Claude Code does — the billing
// header is always the very first system block, before any other system content.
//
// If the body already contains a billing header block, it is replaced with the new one.
//
// The cliVersion parameter should be the version from the fingerprint's User-Agent
// (e.g. "2.1.88"). The entrypoint defaults to "cli".
func injectBillingHeader(body []byte, cliVersion string) []byte {
	if cliVersion == "" {
		return body
	}

	// Extract first user message text for fingerprint computation
	firstMsgText := extractFirstUserMessageText(body)
	fingerprint := computeBillingFingerprint(firstMsgText, cliVersion)
	billingText := buildBillingHeaderText(cliVersion, fingerprint, "cli")

	// Build the billing header block (no cache_control — matches real Claude Code behavior)
	billingBlock, err := marshalAnthropicSystemTextBlock(billingText, false)
	if err != nil {
		logger.LegacyPrintf("service.billing_header", "Warning: failed to marshal billing header block: %v", err)
		return body
	}

	system := gjson.GetBytes(body, "system")

	var items [][]byte

	switch {
	case !system.Exists() || system.Type == gjson.Null:
		// No system prompt: create one with just the billing header
		items = [][]byte{billingBlock}

	case system.Type == gjson.String:
		// String system prompt: convert to array with billing header first
		existingText := system.String()
		if strings.HasPrefix(strings.TrimSpace(existingText), "x-anthropic-billing-header") {
			// Replace existing billing header
			items = [][]byte{billingBlock}
		} else {
			existingBlock, buildErr := marshalAnthropicSystemTextBlock(existingText, false)
			if buildErr != nil {
				return body
			}
			items = [][]byte{billingBlock, existingBlock}
		}

	case system.IsArray():
		// Array system prompt: prepend billing header, remove any existing one
		items = [][]byte{billingBlock}
		system.ForEach(func(_, item gjson.Result) bool {
			// Skip existing billing header blocks
			if item.Get("type").String() == "text" {
				text := item.Get("text").String()
				if strings.HasPrefix(strings.TrimSpace(text), "x-anthropic-billing-header") {
					return true // skip
				}
			}
			items = append(items, []byte(item.Raw))
			return true
		})

	default:
		return body
	}

	result, ok := setJSONRawBytes(body, "system", buildJSONArrayRaw(items))
	if !ok {
		logger.LegacyPrintf("service.billing_header", "Warning: failed to inject billing header into system prompt")
		return body
	}
	return result
}

// systemHasBillingHeader checks if the system prompt already contains a billing header block.
func systemHasBillingHeader(body []byte) bool {
	system := gjson.GetBytes(body, "system")
	if system.Type == gjson.String {
		return strings.HasPrefix(strings.TrimSpace(system.String()), "x-anthropic-billing-header")
	}
	if system.IsArray() {
		hasBilling := false
		system.ForEach(func(_, item gjson.Result) bool {
			if item.Get("type").String() == "text" {
				text := item.Get("text").String()
				if strings.HasPrefix(strings.TrimSpace(text), "x-anthropic-billing-header") {
					hasBilling = true
					return false
				}
			}
			return true
		})
		return hasBilling
	}
	return false
}

// ensureBillingHeader injects the billing header into the body if not already present.
// Uses fingerprintUA to extract the CLI version; falls back to the global current version.
// This is the single entry point used by all forwarding paths (Forward, ForwardAsChatCompletions,
// ForwardAsResponses) to avoid code duplication.
func ensureBillingHeader(body []byte, fingerprintUA string) []byte {
	if systemHasBillingHeader(body) {
		return body
	}
	billingVersion := ExtractCLIVersion(fingerprintUA)
	if billingVersion == "" {
		billingVersion = claude.GetCurrentCLIVersion()
	}
	return injectBillingHeader(body, billingVersion)
}
