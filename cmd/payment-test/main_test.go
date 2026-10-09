package main

import (
	"strings"
	"testing"
	"time"
)

func TestSandboxGuards(t *testing.T) {
	tests := []struct {
		name, key, database, environment string
		wantError                        string
	}{
		{
			name:        "development allowed",
			key:         "xnd_development_synthetic",
			database:    "payment_sandbox_test",
			environment: "development",
		},
		{
			name:        "live forbidden",
			key:         "xnd_production_synthetic",
			database:    "payment_sandbox_test",
			environment: "development",
			wantError:   "live keys are forbidden",
		},
		{
			name:        "ordinary database forbidden",
			key:         "xnd_development_synthetic",
			database:    "payment",
			environment: "development",
			wantError:   "dedicated database",
		},
		{
			name:        "production environment forbidden",
			key:         "xnd_development_synthetic",
			database:    "payment_sandbox_test",
			environment: "production",
			wantError:   "APP_ENV",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("APP_ENV", test.environment)
			t.Setenv("XENDIT_SECRET_KEY", test.key)
			t.Setenv("XENDIT_WEBHOOK_TOKEN", "synthetic-test-token")
			t.Setenv("XENDIT_BASE_URL", "https://api.xendit.co")
			t.Setenv("DATABASE_URL", "postgres://test:test@127.0.0.1:5433/"+test.database)
			opts := options{apiURL: "http://127.0.0.1:8080", timeout: time.Second}
			err := validateConfig(&opts)
			if test.wantError == "" && err != nil {
				t.Fatal(err)
			}
			if test.wantError != "" && (err == nil || !strings.Contains(err.Error(), test.wantError)) {
				t.Fatalf("got %v; want error containing %q", err, test.wantError)
			}
		})
	}
}

func TestActionURLDoesNotExposeCredentials(t *testing.T) {
	got := sanitizedActionURL("https://example.com/pay?api_key=credential&token=private&payment=123")
	containsCredential := strings.Contains(got, "credential&") || strings.Contains(got, "private")
	containsSecretQuery := strings.Contains(got, "api_key=") || strings.Contains(got, "token=")
	if containsCredential || containsSecretQuery {
		t.Fatalf("action URL exposes credentials: %s", got)
	}
	if !strings.Contains(got, "payment=123") {
		t.Fatal("non-secret payment parameter lost")
	}
}
