package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestComputeBillingFingerprint(t *testing.T) {
	// Test with known values: the algorithm is SHA256(salt + chars + version)[:3]
	// salt = "59cf53e54c78"
	// For empty message, all indices use "0": chars = "000"
	fp := computeBillingFingerprint("", "2.1.88")
	assert.Len(t, fp, 3, "fingerprint should be 3 hex characters")

	// Fingerprint should be deterministic
	fp2 := computeBillingFingerprint("", "2.1.88")
	assert.Equal(t, fp, fp2, "same input should produce same fingerprint")

	// Different version should produce different fingerprint
	fp3 := computeBillingFingerprint("", "2.1.89")
	assert.NotEqual(t, fp, fp3, "different version should produce different fingerprint")

	// Test with message text
	msg := "Hello, I need help with my code"
	// msg[4] = 'o', msg[7] = ' ', msg[20] = 'y'
	fp4 := computeBillingFingerprint(msg, "2.1.88")
	assert.Len(t, fp4, 3)

	// Short message (less than 21 chars)
	fpShort := computeBillingFingerprint("Hi", "2.1.88")
	assert.Len(t, fpShort, 3)
}

func TestExtractFirstUserMessageText(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected string
	}{
		{
			name:     "string content",
			body:     `{"messages":[{"role":"user","content":"hello world test message long enough"}]}`,
			expected: "hello world test message long enough",
		},
		{
			name:     "array content with text block",
			body:     `{"messages":[{"role":"user","content":[{"type":"text","text":"hello from array"}]}]}`,
			expected: "hello from array",
		},
		{
			name:     "skip assistant message",
			body:     `{"messages":[{"role":"assistant","content":"I am Claude"},{"role":"user","content":"hello user"}]}`,
			expected: "hello user",
		},
		{
			name:     "empty messages",
			body:     `{"messages":[]}`,
			expected: "",
		},
		{
			name:     "no messages field",
			body:     `{"model":"claude-3"}`,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractFirstUserMessageText([]byte(tt.body))
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildBillingHeaderText(t *testing.T) {
	header := buildBillingHeaderText("2.1.88", "a3f", "cli")
	assert.Equal(t, "x-anthropic-billing-header: cc_version=2.1.88.a3f; cc_entrypoint=cli;", header)
}

func TestInjectBillingHeader(t *testing.T) {
	version := "2.1.88"

	t.Run("inject into body with no system prompt", func(t *testing.T) {
		body := []byte(`{"model":"claude-3","messages":[{"role":"user","content":"hello"}]}`)
		result := injectBillingHeader(body, version)

		system := gjson.GetBytes(result, "system")
		require.True(t, system.IsArray(), "system should be an array")
		require.Equal(t, 1, len(system.Array()), "should have 1 system block")

		text := system.Array()[0].Get("text").String()
		assert.True(t, strings.HasPrefix(text, "x-anthropic-billing-header:"), "should start with billing header prefix")
		assert.Contains(t, text, "cc_version=2.1.88.")
		assert.Contains(t, text, "cc_entrypoint=cli")
	})

	t.Run("inject into body with string system prompt", func(t *testing.T) {
		body := []byte(`{"model":"claude-3","system":"You are a helpful assistant","messages":[{"role":"user","content":"hello"}]}`)
		result := injectBillingHeader(body, version)

		system := gjson.GetBytes(result, "system")
		require.True(t, system.IsArray(), "system should be an array")
		require.Equal(t, 2, len(system.Array()), "should have billing header + original")

		text0 := system.Array()[0].Get("text").String()
		assert.True(t, strings.HasPrefix(text0, "x-anthropic-billing-header:"))

		text1 := system.Array()[1].Get("text").String()
		assert.Equal(t, "You are a helpful assistant", text1)
	})

	t.Run("inject into body with array system prompt", func(t *testing.T) {
		body := []byte(`{"model":"claude-3","system":[{"type":"text","text":"System instruction"}],"messages":[{"role":"user","content":"hello"}]}`)
		result := injectBillingHeader(body, version)

		system := gjson.GetBytes(result, "system")
		require.True(t, system.IsArray())
		require.Equal(t, 2, len(system.Array()))

		text0 := system.Array()[0].Get("text").String()
		assert.True(t, strings.HasPrefix(text0, "x-anthropic-billing-header:"))
	})

	t.Run("replace existing billing header", func(t *testing.T) {
		body := []byte(`{"model":"claude-3","system":[{"type":"text","text":"x-anthropic-billing-header: old"},{"type":"text","text":"System instruction"}],"messages":[{"role":"user","content":"hello"}]}`)
		result := injectBillingHeader(body, version)

		system := gjson.GetBytes(result, "system")
		require.True(t, system.IsArray())
		require.Equal(t, 2, len(system.Array()))

		text0 := system.Array()[0].Get("text").String()
		assert.True(t, strings.HasPrefix(text0, "x-anthropic-billing-header: cc_version="))
		assert.NotEqual(t, "x-anthropic-billing-header: old", text0)
	})

	t.Run("empty version returns body unchanged", func(t *testing.T) {
		body := []byte(`{"model":"claude-3","messages":[{"role":"user","content":"hello"}]}`)
		result := injectBillingHeader(body, "")
		assert.Equal(t, body, result)
	})
}

func TestSystemHasBillingHeader(t *testing.T) {
	assert.True(t, systemHasBillingHeader([]byte(`{"system":"x-anthropic-billing-header: keep"}`)))
	assert.True(t, systemHasBillingHeader([]byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: keep"}]}`)))
	assert.False(t, systemHasBillingHeader([]byte(`{"system":"You are helpful"}`)))
	assert.False(t, systemHasBillingHeader([]byte(`{"system":[{"type":"text","text":"You are helpful"}]}`)))
	assert.False(t, systemHasBillingHeader([]byte(`{}`)))
}
