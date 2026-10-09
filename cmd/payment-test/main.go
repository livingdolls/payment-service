// payment-test performs opt-in real Xendit sandbox verification. Ordinary
// go tests never invoke this command or contact Xendit.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/livingdolls/payment-service/internal/testkit"
)

type options struct {
	apiURL      string
	fixture     string
	reportDir   string
	channels    string
	timeout     time.Duration
	checkOnly   bool
	listOnly    bool
	secretKey   string
	databaseURL string
}

func main() {
	os.Exit(run())
}

func run() int {
	var opts options
	apiURL := os.Getenv("SANDBOX_API_URL")
	if apiURL == "" {
		apiURL = "http://127.0.0.1:8091"
	}
	flag.StringVar(
		&opts.apiURL,
		"api-url",
		apiURL,
		"running local payment API",
	)
	flag.StringVar(
		&opts.fixture,
		"fixture",
		"",
		"channel fixture; defaults to repository testdata",
	)
	flag.StringVar(
		&opts.reportDir,
		"report-dir",
		filepath.Join("artifacts", "payment-tests"),
		"sanitized JSON and Markdown report directory",
	)
	flag.StringVar(
		&opts.channels,
		"channel",
		"",
		"comma-separated channel codes; default all catalog entries",
	)
	flag.DurationVar(
		&opts.timeout,
		"timeout",
		60*time.Second,
		"maximum completion wait per channel",
	)
	flag.BoolVar(
		&opts.checkOnly,
		"check-config",
		false,
		"validate sandbox guards without any network request",
	)
	flag.BoolVar(
		&opts.listOnly,
		"list",
		false,
		"print catalog eligibility without network requests",
	)
	flag.Parse()
	profiles, err := testkit.LoadChannels(opts.fixture)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Unable to load channel fixture.")
		return 2
	}
	if opts.listOnly {
		for _, profile := range profiles {
			fmt.Printf(
				"%s\t%s\t%s\n",
				profile.ChannelCode,
				profile.CompletionMode,
				profile.Reason,
			)
		}
		return 0
	}
	if err := validateConfig(&opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if opts.checkOnly {
		fmt.Println("Sandbox configuration valid; no payment or external request made.")
		return 0
	}
	if os.Getenv("SANDBOX_CALLBACK_CONFIGURED") != "1" {
		reason := "Payment Requests v3 – Payment Status callback in TEST mode has not been confirmed; " +
			"configure SANDBOX_PUBLIC_URL/webhooks/xendit/payments using the existing XENDIT_WEBHOOK_TOKEN, " +
			"then set SANDBOX_CALLBACK_CONFIGURED=1."
		report := testkit.Report{Mode: "sandbox", StartedAt: time.Now().UTC()}
		for _, profile := range profiles {
			blockReason := reason
			if !profile.Eligible {
				blockReason = profile.Reason
			}
			report.Results = append(report.Results, testkit.ChannelResult{
				ChannelCode: profile.ChannelCode,
				Status:      "BLOCKED",
				Reason:      blockReason,
				SourceURLs:  profile.SourceURLs,
			})
		}
		report.Finalize()
		jsonPath, markdownPath, err := testkit.WriteReport(opts.reportDir, report)
		if err == nil {
			fmt.Printf("Blocked sandbox reports: %s and %s\n", jsonPath, markdownPath)
		}
		fmt.Fprintln(os.Stderr, "BLOCKED: "+reason)
		return 2
	}
	if err := validatePublicURL(os.Getenv("SANDBOX_PUBLIC_URL")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	selected, err := selectedChannels(opts.channels, profiles)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	poolConfig, err := pgxpool.ParseConfig(opts.databaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Invalid sandbox database configuration.")
		return 2
	}
	poolConfig.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	poolConfig.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Cannot connect to sandbox verification database.")
		return 2
	}
	defer pool.Close()
	checkCtx, checkCancel := context.WithTimeout(ctx, 5*time.Second)
	err = pool.Ping(checkCtx)
	checkCancel()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Sandbox verification database is unavailable.")
		return 2
	}
	client := &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	runner := &sandboxRunner{opts: opts, db: pool, client: client}
	if !runner.ready(ctx) {
		fmt.Fprintln(os.Stderr, "Payment API readiness check failed.")
		return 2
	}
	report := testkit.Report{Mode: "sandbox", StartedAt: time.Now().UTC()}
	for _, profile := range profiles {
		result := testkit.ChannelResult{
			ChannelCode: profile.ChannelCode,
			Amount:      profile.Amount,
			SourceURLs:  profile.SourceURLs,
		}
		switch {
		case !profile.Eligible:
			result.Status, result.Reason = "BLOCKED", profile.Reason
		case !selected[profile.ChannelCode]:
			result.Status, result.Reason = "SKIP", "Not selected for this run."
		case ctx.Err() != nil:
			result.Status, result.Reason = "SKIP", "Run interrupted before this channel started."
		default:
			result = runner.testChannel(ctx, profile)
		}
		report.Results = append(report.Results, result)
		fmt.Printf("%s %s: %s\n", result.Status, result.ChannelCode, result.Reason)
	}
	report.Finalize()
	jsonPath, markdownPath, err := testkit.WriteReport(opts.reportDir, report)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Unable to write sanitized test report.")
		return 2
	}
	fmt.Printf(
		"Reports: %s and %s\nPASS=%d FAIL=%d SKIP=%d BLOCKED=%d\n",
		jsonPath,
		markdownPath,
		report.Counts["PASS"],
		report.Counts["FAIL"],
		report.Counts["SKIP"],
		report.Counts["BLOCKED"],
	)
	if report.Counts["FAIL"] > 0 {
		return 1
	}
	eligible := make(map[string]bool)
	selectedEligible := 0
	for _, profile := range profiles {
		eligible[profile.ChannelCode] = profile.Eligible
		if selected[profile.ChannelCode] && profile.Eligible {
			selectedEligible++
		}
	}
	if selectedEligible == 0 {
		return 2
	}
	for _, result := range report.Results {
		if selected[result.ChannelCode] && eligible[result.ChannelCode] && result.Status == "BLOCKED" {
			return 2
		}
	}
	if ctx.Err() != nil {
		return 2
	}
	return 0
}

func validateConfig(opts *options) error {
	environment := os.Getenv("APP_ENV")
	if environment != "development" && environment != "test" {
		return fmt.Errorf("sandbox runner requires APP_ENV=development or test")
	}
	opts.secretKey = os.Getenv("XENDIT_SECRET_KEY")
	if !strings.HasPrefix(opts.secretKey, "xnd_development_") && !strings.HasPrefix(opts.secretKey, "xnd_test_") {
		return fmt.Errorf("sandbox runner requires a development/test Xendit secret key; live keys are forbidden")
	}
	base := os.Getenv("XENDIT_BASE_URL")
	if base != "" && strings.TrimRight(base, "/") != "https://api.xendit.co" {
		return fmt.Errorf("sandbox runner requires the official HTTPS Xendit API endpoint")
	}
	if os.Getenv("XENDIT_WEBHOOK_TOKEN") == "" {
		return fmt.Errorf("XENDIT_WEBHOOK_TOKEN is required")
	}
	api, err := url.Parse(opts.apiURL)
	if err != nil {
		return fmt.Errorf("payment API must use a local loopback URL")
	}
	localAPI := api.Hostname() == "127.0.0.1" || api.Hostname() == "localhost" || api.Hostname() == "::1"
	validScheme := api.Scheme == "http" || api.Scheme == "https"
	plainURL := api.User == nil && api.RawQuery == "" && api.Fragment == ""
	if !localAPI || !validScheme || !plainURL {
		return fmt.Errorf("payment API must use a local loopback URL")
	}
	opts.apiURL = strings.TrimRight(opts.apiURL, "/")
	opts.databaseURL = os.Getenv("DATABASE_URL")
	config, err := pgxpool.ParseConfig(opts.databaseURL)
	if err != nil || opts.databaseURL == "" {
		return fmt.Errorf("DATABASE_URL must identify a dedicated database whose name ends in _test")
	}
	dedicatedDatabase := regexp.MustCompile(`^[a-z][a-z0-9_]{0,57}_test$`).MatchString(config.ConnConfig.Database)
	if !dedicatedDatabase {
		return fmt.Errorf("DATABASE_URL must identify a dedicated database whose name ends in _test")
	}
	if opts.timeout <= 0 || opts.timeout > 30*time.Minute {
		return fmt.Errorf("timeout must be greater than zero and at most 30 minutes")
	}
	return nil
}

func validatePublicURL(raw string) error {
	public, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("SANDBOX_PUBLIC_URL must be the public HTTPS tunnel URL")
	}
	publicHTTPS := public.Scheme == "https" && public.Host != ""
	plainURL := public.User == nil && public.RawQuery == "" && public.Fragment == ""
	if !publicHTTPS || !plainURL {
		return fmt.Errorf("SANDBOX_PUBLIC_URL must be the public HTTPS tunnel URL")
	}
	return nil
}

func selectedChannels(raw string, profiles []testkit.ChannelProfile) (map[string]bool, error) {
	known := make(map[string]bool)
	for _, profile := range profiles {
		known[profile.ChannelCode] = true
	}
	if strings.TrimSpace(raw) == "" {
		return known, nil
	}
	selected := make(map[string]bool)
	for _, code := range strings.Split(raw, ",") {
		code = strings.ToUpper(strings.TrimSpace(code))
		if !known[code] {
			return nil, fmt.Errorf("unknown channel code %q", code)
		}
		selected[code] = true
	}
	return selected, nil
}
