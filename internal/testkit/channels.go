// Package testkit shares payment test profiles and reports between the local
// integration suite and the opt-in Xendit sandbox runner.
package testkit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
)

type ChannelProfile struct {
	ChannelCode    string         `json:"channel_code"`
	Eligible       bool           `json:"eligible"`
	CompletionMode string         `json:"completion_mode"`
	Amount         int64          `json:"amount"`
	Properties     map[string]any `json:"properties"`
	Reason         string         `json:"reason,omitempty"`
	SourceURLs     []string       `json:"source_urls"`
	EvidenceURLs   []string       `json:"-"`
}

// LoadChannels loads the reviewed IDR catalog. An empty path locates the
// repository fixture independently of the caller's working directory.
func LoadChannels(path string) ([]ChannelProfile, error) {
	if path == "" {
		_, source, _, ok := runtime.Caller(0)
		if !ok {
			return nil, fmt.Errorf("locate channel fixture")
		}
		path = filepath.Join(
			filepath.Dir(source),
			"..",
			"..",
			"testdata",
			"xendit-idr-channels.json",
		)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read channel fixture: %w", err)
	}
	var fixture struct {
		SchemaVersion int              `json:"schema_version"`
		Channels      []ChannelProfile `json:"channels"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		return nil, fmt.Errorf("decode channel fixture: %w", err)
	}
	if fixture.SchemaVersion != 1 || len(fixture.Channels) == 0 {
		return nil, fmt.Errorf("unsupported or empty channel fixture")
	}
	seen := make(map[string]bool)
	for i := range fixture.Channels {
		channel := &fixture.Channels[i]
		if channel.ChannelCode == "" || seen[channel.ChannelCode] || len(channel.SourceURLs) == 0 {
			return nil, fmt.Errorf("invalid or duplicate channel profile at index %d", i)
		}
		seen[channel.ChannelCode] = true
		if channel.Eligible {
			if channel.Amount <= 0 || (channel.CompletionMode != "simulate" && channel.CompletionMode != "actions") {
				return nil, fmt.Errorf("invalid runnable profile: %s", channel.ChannelCode)
			}
			if channel.ChannelCode == "JENIUSPAY" {
				cashtag, ok := channel.Properties["cashtag"].(string)
				validCashtag := ok && regexp.MustCompile(`^[$][a-zA-Z0-9_]{3,15}$`).MatchString(cashtag)
				if !validCashtag {
					return nil, fmt.Errorf("JENIUSPAY profile requires a valid $cashtag")
				}
			}
		} else if channel.CompletionMode != "blocked" || channel.Reason == "" {
			return nil, fmt.Errorf("blocked profile requires reason: %s", channel.ChannelCode)
		}
		channel.EvidenceURLs = channel.SourceURLs
	}
	return fixture.Channels, nil
}
