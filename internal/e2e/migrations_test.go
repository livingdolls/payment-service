//go:build integration

package e2e_test

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestEmptyDatabaseMigrationRoundtrip(t *testing.T) {
	db := isolatedDatabase(t)
	var schema string
	if err := db.QueryRow(t.Context(), "SELECT current_schema()").Scan(&schema); err != nil {
		t.Fatal(err)
	}
	goose, err := exec.LookPath("goose")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	dsn := os.Getenv("TEST_DATABASE_URL")
	constraintDefinition := func() string {
		t.Helper()
		var definition string
		const query = `
			SELECT pg_get_constraintdef(oid)
			FROM pg_constraint
			WHERE conrelid='payment_attempts'::regclass
				AND conname='chk_payment_attempts_status'
		`
		if err := db.QueryRow(ctx, query).Scan(&definition); err != nil {
			t.Fatal(err)
		}
		return definition
	}
	// Inspect migration 5's rollback before continuing to an entirely empty
	// domain schema: it must restore migration 3's original status constraint.
	runGoose(
		t,
		ctx,
		goose,
		dsn,
		schema,
		"down-to",
		"4",
	)
	restored := constraintDefinition()
	originalStatusesPresent := strings.Contains(restored, "'CREATED'") && strings.Contains(restored, "'UNKNOWN'")
	additionalStatusesPresent := strings.Contains(restored, "'PENDING'") || strings.Contains(restored, "'CANCELED'")
	if !originalStatusesPresent || additionalStatusesPresent {
		t.Fatalf("migration 5 rollback did not restore the original status constraint: %s", restored)
	}
	runGoose(
		t,
		ctx,
		goose,
		dsn,
		schema,
		"down-to",
		"0",
	)
	var tables int
	const countTables = `
		SELECT count(*) FROM pg_tables
		WHERE schemaname=$1 AND tablename <> 'goose_db_version'
	`
	if err := db.QueryRow(ctx, countTables, schema).Scan(&tables); err != nil || tables != 0 {
		t.Fatalf("rollback left %d domain tables in isolated schema: %v", tables, err)
	}
	runGoose(
		t,
		ctx,
		goose,
		dsn,
		schema,
		"up",
	)
	expanded := constraintDefinition()
	if !strings.Contains(expanded, "'PENDING'") || !strings.Contains(expanded, "'CANCELED'") {
		t.Fatalf("roundtrip did not restore expanded status constraint: %s", expanded)
	}
	var version int64
	var applied bool
	const latestMigration = `
		SELECT version_id,is_applied FROM goose_db_version
		ORDER BY id DESC LIMIT 1
	`
	row := db.QueryRow(ctx, latestMigration)
	if err := row.Scan(&version, &applied); err != nil || version != 10 || !applied {
		t.Fatalf(
			"roundtrip migration version=%d applied=%t error=%v",
			version,
			applied,
			err,
		)
	}
}
