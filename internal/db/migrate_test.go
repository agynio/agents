package db

import (
	"context"
	"io/fs"
	"os"
	"sort"
	"testing"

	"github.com/agynio/agents/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The migrations under test. Each derives an organization for every row already
// in its table from the target that row carries, so reads can be scoped by it.
const (
	envOrganizationScopeMigration                       = "0016_env_organization_scope.sql"
	imagePullSecretAttachmentOrganizationScopeMigration = "0017_image_pull_secret_attachment_organization_scope.sql"
)

const (
	organizationOne = "11111111-1111-1111-1111-111111111111"
	organizationTwo = "22222222-2222-2222-2222-222222222222"
)

// migrationTestPool connects to the database named by
// AGENTS_MIGRATION_TEST_DATABASE_URL and empties it first. The variable must
// name a scratch database, never one holding anything worth keeping. It is
// unset in ordinary runs and these tests are then skipped, so the package needs
// no database to be tested.
func migrationTestPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("AGENTS_MIGRATION_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("AGENTS_MIGRATION_TEST_DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	return pool
}

func migrationNames(t *testing.T) []string {
	t.Helper()
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names
}

// applyMigrationsBefore brings the database to the state the migration under
// test will find, recording what it applied the way ApplyMigrations does so the
// real entry point picks up from there.
func applyMigrationsBefore(ctx context.Context, t *testing.T, pool *pgxpool.Pool, version string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`); err != nil {
		t.Fatalf("ensure schema_migrations: %v", err)
	}
	for _, name := range migrationNames(t) {
		if name >= version {
			return
		}
		content, err := migrations.Files.ReadFile(name)
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, string(content)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, name); err != nil {
			t.Fatalf("record migration %s: %v", name, err)
		}
	}
}

func execAll(ctx context.Context, t *testing.T, pool *pgxpool.Pool, statements []string) {
	t.Helper()
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

// seedTargetsAcrossOrganizations writes the agents, mcps and hooks a scoped row
// can point at, deliberately putting the mcp in a different organization from
// the agent and the hook so a backfill that guessed a single organization would
// be caught.
func seedTargetsAcrossOrganizations(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	execAll(ctx, t, pool, []string{
		`INSERT INTO agents (id, organization_id, name, model, init_image) VALUES
			('aaaaaaaa-0000-0000-0000-000000000001', '` + organizationOne + `', 'one', uuid_generate_v4(), 'init:1'),
			('aaaaaaaa-0000-0000-0000-000000000002', '` + organizationTwo + `', 'two', uuid_generate_v4(), 'init:1')`,
		`INSERT INTO mcps (id, agent_id, name, image) VALUES
			('bbbbbbbb-0000-0000-0000-000000000001', 'aaaaaaaa-0000-0000-0000-000000000002', 'mcp_one', 'mcp:1')`,
		`INSERT INTO hooks (id, agent_id, event, image) VALUES
			('cccccccc-0000-0000-0000-000000000001', 'aaaaaaaa-0000-0000-0000-000000000001', 'pre', 'hook:1')`,
	})
}

// seedEnvsAcrossOrganizations writes one env per target kind.
func seedEnvsAcrossOrganizations(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	seedTargetsAcrossOrganizations(ctx, t, pool)
	execAll(ctx, t, pool, []string{
		`INSERT INTO envs (name, agent_id, value) VALUES ('BY_AGENT', 'aaaaaaaa-0000-0000-0000-000000000001', 'v')`,
		`INSERT INTO envs (name, mcp_id, value) VALUES ('BY_MCP', 'bbbbbbbb-0000-0000-0000-000000000001', 'v')`,
		`INSERT INTO envs (name, hook_id, secret_id) VALUES ('BY_HOOK', 'cccccccc-0000-0000-0000-000000000001', uuid_generate_v4())`,
	})
}

// seedImagePullSecretAttachmentsAcrossOrganizations writes one attachment per
// target kind. The rows carry no name, so each is identified by a fixed id.
func seedImagePullSecretAttachmentsAcrossOrganizations(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	seedTargetsAcrossOrganizations(ctx, t, pool)
	execAll(ctx, t, pool, []string{
		`INSERT INTO image_pull_secret_attachments (id, image_pull_secret_id, agent_id) VALUES
			('dddddddd-0000-0000-0000-000000000001', uuid_generate_v4(), 'aaaaaaaa-0000-0000-0000-000000000001')`,
		`INSERT INTO image_pull_secret_attachments (id, image_pull_secret_id, mcp_id) VALUES
			('dddddddd-0000-0000-0000-000000000002', uuid_generate_v4(), 'bbbbbbbb-0000-0000-0000-000000000001')`,
		`INSERT INTO image_pull_secret_attachments (id, image_pull_secret_id, hook_id) VALUES
			('dddddddd-0000-0000-0000-000000000003', uuid_generate_v4(), 'cccccccc-0000-0000-0000-000000000001')`,
	})
}

func TestEnvOrganizationBackfillFollowsTheTargetsParentChain(t *testing.T) {
	ctx := context.Background()
	pool := migrationTestPool(ctx, t)
	applyMigrationsBefore(ctx, t, pool, envOrganizationScopeMigration)
	seedEnvsAcrossOrganizations(ctx, t, pool)

	if err := ApplyMigrations(ctx, pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	expected := map[string]string{
		"BY_AGENT": organizationOne,
		// An mcp and a hook carry an agent rather than an organization, so the
		// backfill has to go through it to reach one.
		"BY_MCP":  organizationTwo,
		"BY_HOOK": organizationOne,
	}
	for name, organizationID := range expected {
		var actual string
		if err := pool.QueryRow(ctx, `SELECT organization_id::text FROM envs WHERE name = $1`, name).Scan(&actual); err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if actual != organizationID {
			t.Fatalf("expected %s in organization %s, got %s", name, organizationID, actual)
		}
	}
}

// An env that reaches no organization is corruption, and the migration stops
// rather than deleting the row or inventing an organization for it. Migrations
// apply in one transaction, so stopping leaves the database as it was.
func TestEnvOrganizationBackfillStopsOnAnEnvWithNoOrganization(t *testing.T) {
	ctx := context.Background()
	pool := migrationTestPool(ctx, t)
	applyMigrationsBefore(ctx, t, pool, envOrganizationScopeMigration)
	seedEnvsAcrossOrganizations(ctx, t, pool)
	if _, err := pool.Exec(ctx, `ALTER TABLE envs DROP CONSTRAINT envs_check`); err != nil {
		t.Fatalf("drop target check: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO envs (name, value) VALUES ('ORPHAN', 'v')`); err != nil {
		t.Fatalf("seed orphan: %v", err)
	}

	if err := ApplyMigrations(ctx, pool); err == nil {
		t.Fatal("expected the migration to stop on an env with no organization")
	}

	var columns int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.columns WHERE table_name = 'envs' AND column_name = 'organization_id'`,
	).Scan(&columns); err != nil {
		t.Fatalf("inspect envs: %v", err)
	}
	if columns != 0 {
		t.Fatal("expected the migration to roll back and leave envs untouched")
	}
}

func TestImagePullSecretAttachmentOrganizationBackfillFollowsTheTargetsParentChain(t *testing.T) {
	ctx := context.Background()
	pool := migrationTestPool(ctx, t)
	applyMigrationsBefore(ctx, t, pool, imagePullSecretAttachmentOrganizationScopeMigration)
	seedImagePullSecretAttachmentsAcrossOrganizations(ctx, t, pool)

	if err := ApplyMigrations(ctx, pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	expected := map[string]string{
		"dddddddd-0000-0000-0000-000000000001": organizationOne,
		// An mcp and a hook carry an agent rather than an organization, so the
		// backfill has to go through it to reach one.
		"dddddddd-0000-0000-0000-000000000002": organizationTwo,
		"dddddddd-0000-0000-0000-000000000003": organizationOne,
	}
	for id, organizationID := range expected {
		var actual string
		if err := pool.QueryRow(ctx, `SELECT organization_id::text FROM image_pull_secret_attachments WHERE id = $1`, id).Scan(&actual); err != nil {
			t.Fatalf("read %s: %v", id, err)
		}
		if actual != organizationID {
			t.Fatalf("expected %s in organization %s, got %s", id, organizationID, actual)
		}
	}
}

// An attachment that reaches no organization is corruption, and the migration
// stops rather than deleting the row or inventing an organization for it.
// Migrations apply in one transaction, so stopping leaves the database as it
// was.
func TestImagePullSecretAttachmentOrganizationBackfillStopsOnAnAttachmentWithNoOrganization(t *testing.T) {
	ctx := context.Background()
	pool := migrationTestPool(ctx, t)
	applyMigrationsBefore(ctx, t, pool, imagePullSecretAttachmentOrganizationScopeMigration)
	seedImagePullSecretAttachmentsAcrossOrganizations(ctx, t, pool)
	if _, err := pool.Exec(ctx, `ALTER TABLE image_pull_secret_attachments DROP CONSTRAINT image_pull_secret_attachments_check`); err != nil {
		t.Fatalf("drop target check: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO image_pull_secret_attachments (image_pull_secret_id) VALUES (uuid_generate_v4())`); err != nil {
		t.Fatalf("seed orphan: %v", err)
	}

	if err := ApplyMigrations(ctx, pool); err == nil {
		t.Fatal("expected the migration to stop on an attachment with no organization")
	}

	var columns int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.columns WHERE table_name = 'image_pull_secret_attachments' AND column_name = 'organization_id'`,
	).Scan(&columns); err != nil {
		t.Fatalf("inspect image_pull_secret_attachments: %v", err)
	}
	if columns != 0 {
		t.Fatal("expected the migration to roll back and leave image_pull_secret_attachments untouched")
	}
}
