package testkit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ChannelResult struct {
	ChannelCode              string   `json:"channel_code"`
	Status                   string   `json:"status"`
	Reason                   string   `json:"reason,omitempty"`
	PaymentID                string   `json:"payment_id,omitempty"`
	AttemptID                string   `json:"attempt_id,omitempty"`
	ProviderPaymentRequestID string   `json:"provider_payment_request_id,omitempty"`
	PaymentStatus            string   `json:"payment_status,omitempty"`
	AttemptStatus            string   `json:"attempt_status,omitempty"`
	WebhookStatus            string   `json:"webhook_status,omitempty"`
	CompletionPath           string   `json:"completion_path,omitempty"`
	Amount                   int64    `json:"amount,omitempty"`
	CapturedAmount           int64    `json:"captured_amount,omitempty"`
	SourceURLs               []string `json:"source_urls,omitempty"`
	EvidenceURLs             []string `json:"-"`
}

type Report struct {
	Mode       string          `json:"mode"`
	StartedAt  time.Time       `json:"started_at"`
	FinishedAt time.Time       `json:"finished_at"`
	Results    []ChannelResult `json:"results"`
	Counts     map[string]int  `json:"counts"`
}

func (r *Report) Finalize() {
	r.FinishedAt = time.Now().UTC()
	r.Counts = map[string]int{"PASS": 0, "FAIL": 0, "SKIP": 0, "BLOCKED": 0}
	for _, result := range r.Results {
		r.Counts[result.Status]++
	}
}

// WriteReport stores only verification results, never request/response payloads,
// credentials, card details, or authorization/action URLs.
func WriteReport(dir string, report Report) (string, string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", fmt.Errorf("create report directory: %w", err)
	}
	name := "payment-" + report.Mode + "-" + report.StartedAt.UTC().Format("20060102T150405.000000000Z")
	jsonPath := filepath.Join(dir, name+".json")
	markdownPath := filepath.Join(dir, name+".md")
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("encode report: %w", err)
	}
	if err := os.WriteFile(jsonPath, append(data, '\n'), 0o600); err != nil {
		return "", "", fmt.Errorf("write JSON report: %w", err)
	}
	var markdown strings.Builder
	fmt.Fprintf(
		&markdown,
		"# Payment verification — %s\n\nRun: %s\n\nPASS: %d · FAIL: %d · SKIP: %d · BLOCKED: %d\n\n",
		report.Mode,
		report.StartedAt.UTC().Format(time.RFC3339),
		report.Counts["PASS"],
		report.Counts["FAIL"],
		report.Counts["SKIP"],
		report.Counts["BLOCKED"],
	)
	markdown.WriteString("| Channel | Result | Payment | Attempt | Webhook | Completion | Reason | Evidence |\n" +
		"| --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, result := range report.Results {
		evidence := make([]string, 0, len(result.SourceURLs))
		for i, source := range result.SourceURLs {
			evidence = append(evidence, fmt.Sprintf("[source %d](%s)", i+1, source))
		}
		fmt.Fprintf(
			&markdown,
			"| %s | %s | %s | %s | %s | %s | %s | %s |\n",
			escapeCell(result.ChannelCode),
			escapeCell(result.Status),
			escapeCell(result.PaymentStatus),
			escapeCell(result.AttemptStatus),
			escapeCell(result.WebhookStatus),
			escapeCell(result.CompletionPath),
			escapeCell(result.Reason),
			strings.Join(evidence, ", "),
		)
	}
	markdown.WriteString("\nLocal results verify application behavior against a mock provider; " +
		"they do not prove merchant activation or Xendit sandbox compatibility. " +
		"Sandbox PASS requires captured payment and attempt, exact captured amount, " +
		"and a processed matching capture webhook.\n")
	if err := os.WriteFile(markdownPath, []byte(markdown.String()), 0o600); err != nil {
		return "", "", fmt.Errorf("write Markdown report: %w", err)
	}
	return jsonPath, markdownPath, nil
}

func escapeCell(value string) string {
	return strings.NewReplacer(
		"|",
		"\\|",
		"\n",
		" ",
		"\r",
		" ",
	).Replace(value)
}
