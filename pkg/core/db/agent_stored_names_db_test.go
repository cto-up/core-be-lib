//go:build testutils

package db_test

import (
	"context"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ctoup.com/coreapp/pkg/core/db/migration"
	"ctoup.com/coreapp/pkg/core/db/testutils"
	connectionRepository "ctoup.com/coreapp/pkg/shared/repository"
)

const agentStoredNamesVersion int64 = 2026103007452801

// hub#652: a tenant whose features say {"aiemployee": true} and whose config
// holds aiemployee.* keys reads {"agent": true} and agent.* after the
// migration, and the old names again after its Down.
func TestAgentStoredNamesMigration_Integration(t *testing.T) {
	pool, cleanup, err := testutils.SetupPostgresContainer()
	require.NoError(t, err)
	t.Cleanup(cleanup)
	sqlDB, err := connectionRepository.ConnectDB(testutils.DB_CONNECTION)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	prov, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migration.FS)
	require.NoError(t, err)
	ctx := context.Background()

	_, err = prov.Up(ctx)
	require.NoError(t, err)
	_, err = prov.DownTo(ctx, agentStoredNamesVersion-1)
	require.NoError(t, err)

	exec := func(sql string) {
		t.Helper()
		_, err := pool.Exec(ctx, sql)
		require.NoError(t, err)
	}
	exec(`INSERT INTO core_tenants (tenant_id, name, subdomain, allow_password_sign_up, user_id, features, feature_licenses)
	      VALUES ('t1', 'T1', 't1', false, 'u1', '{"aiemployee": true, "workflow": false}', '{"aiemployee": 3}')`)
	exec(`INSERT INTO core_users (id) VALUES ('u1')`)
	exec(`INSERT INTO core_user_tenant_memberships (user_id, tenant_id, joined_at, feature_licenses)
	      VALUES ('u1', 't1', now(), '{"aiemployee": true}')`)
	exec(`INSERT INTO core_tenant_configs (name, value, user_id, tenant_id) VALUES
	      ('aiemployee.memory.episodic_ttl_days', '30', 'u1', 't1'),
	      ('scholar.model', 'x', 'u1', 't1')`)
	exec(`INSERT INTO core_personal_data_access_log (tenant_id, actor_user_id, subject_user_id, resource)
	      VALUES ('t1', 'a', 's', 'aiemployee.conversation')`)

	type state struct{ Features, TenantLicenses, MemberLicenses, Configs, Resource string }
	read := func() state {
		t.Helper()
		var s state
		require.NoError(t, pool.QueryRow(ctx, `
			SELECT (SELECT features::text FROM core_tenants WHERE tenant_id = 't1'),
			       (SELECT feature_licenses::text FROM core_tenants WHERE tenant_id = 't1'),
			       (SELECT feature_licenses::text FROM core_user_tenant_memberships WHERE tenant_id = 't1'),
			       (SELECT string_agg(name || '=' || value, ',' ORDER BY name) FROM core_tenant_configs WHERE tenant_id = 't1'),
			       (SELECT resource FROM core_personal_data_access_log WHERE tenant_id = 't1')`).
			Scan(&s.Features, &s.TenantLicenses, &s.MemberLicenses, &s.Configs, &s.Resource))
		return s
	}

	_, err = prov.UpTo(ctx, agentStoredNamesVersion)
	require.NoError(t, err)
	assert.Equal(t, state{
		Features:       `{"agent": true, "workflow": false}`,
		TenantLicenses: `{"agent": 3}`,
		MemberLicenses: `{"agent": true}`,
		Configs:        "agent.memory.episodic_ttl_days=30,scholar.model=x",
		Resource:       "agent.conversation",
	}, read())

	_, err = prov.DownTo(ctx, agentStoredNamesVersion-1)
	require.NoError(t, err)
	assert.Equal(t, state{
		Features:       `{"workflow": false, "aiemployee": true}`,
		TenantLicenses: `{"aiemployee": 3}`,
		MemberLicenses: `{"aiemployee": true}`,
		Configs:        "aiemployee.memory.episodic_ttl_days=30,scholar.model=x",
		Resource:       "aiemployee.conversation",
	}, read())
}
