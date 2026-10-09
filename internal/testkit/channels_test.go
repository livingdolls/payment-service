package testkit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Enforce the provider's documented profile contract before the permissive
// local provider mock can turn an invalid sandbox profile into a false pass.
func TestLoadChannelsJeniusCashtag(t *testing.T) {
	tests := []struct {
		name      string
		cashtag   string
		wantError bool
	}{
		{name: "valid syntax", cashtag: "$paymenttest"},
		{name: "missing dollar prefix", cashtag: "paymenttest", wantError: true},
		{name: "too short", cashtag: "$ab", wantError: true},
		{name: "too long", cashtag: "$abcdefghijklmnop", wantError: true},
		{name: "invalid punctuation", cashtag: "$test-name", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fixture := struct {
				SchemaVersion int              `json:"schema_version"`
				Channels      []ChannelProfile `json:"channels"`
			}{
				SchemaVersion: 1,
				Channels: []ChannelProfile{{
					ChannelCode:    "JENIUSPAY",
					Eligible:       true,
					CompletionMode: "actions",
					Amount:         50000,
					Properties:     map[string]any{"cashtag": test.cashtag},
					SourceURLs:     []string{"https://docs.xendit.co/docs/jeniuspay"},
				}},
			}
			data, err := json.Marshal(fixture)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "channels.json")
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			_, err = LoadChannels(path)
			if !test.wantError && err != nil {
				t.Fatal(err)
			}
			if test.wantError && (err == nil || !strings.Contains(err.Error(), "$cashtag")) {
				t.Fatalf("got %v; want invalid $cashtag error", err)
			}
		})
	}
}
