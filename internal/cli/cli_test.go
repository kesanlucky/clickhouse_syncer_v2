package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"clickhouse-syncer/internal/config"
)

func testConfig() *config.Config {
	return &config.Config{
		Tables: []config.TableConfig{
			{Name: "db.table1", DateColumn: "time"},
			{Name: "db.table2", DateColumn: "created_at"},
		},
	}
}

func TestResolveSelectedTables(t *testing.T) {
	cfg := testConfig()

	// Mutual exclusion
	_, err := resolveSelectedTables(cfg, "db.table1", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")

	// Table found
	tables, err := resolveSelectedTables(cfg, "db.table1", false)
	require.NoError(t, err)
	assert.Len(t, tables, 1)
	assert.Equal(t, "db.table1", tables[0].Name)

	// Table not found
	_, err = resolveSelectedTables(cfg, "db.table3", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Defaults to all
	tables, err = resolveSelectedTables(cfg, "", true)
	require.NoError(t, err)
	assert.Len(t, tables, 2)

	tables, err = resolveSelectedTables(cfg, "", false)
	require.NoError(t, err)
	assert.Len(t, tables, 2)
}

func TestNewRootCommand(t *testing.T) {
	cmd := NewRootCommand()
	assert.Equal(t, "clickhouse-syncer", cmd.Use)
	assert.True(t, cmd.HasSubCommands())

	subcommands := cmd.Commands()
	names := make([]string, 0, len(subcommands))
	for _, c := range subcommands {
		names = append(names, c.Name())
	}

	assert.Contains(t, names, "sync")
	assert.Contains(t, names, "delete")
	assert.Contains(t, names, "dry-run")
	assert.Contains(t, names, "validate")
	assert.Contains(t, names, "standby-sync")

	assert.NotNil(t, cmd.PersistentFlags().Lookup("config"))
}
