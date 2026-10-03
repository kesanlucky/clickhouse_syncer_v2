package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTempConfig(t *testing.T, content string) string {
	f, err := os.CreateTemp("", "config-*.yaml")
	require.NoError(t, err)
	_, err = f.WriteString(content)
	require.NoError(t, err)
	f.Close()
	return f.Name()
}

func TestLoad_ValidConfig(t *testing.T) {
	content := `
source:
  host: "src.local"
  database: "default"
destination:
  host: "dest.local"
  database: "default"
tables:
  - name: "table1"
    date_column: "date"
`
	path := writeTempConfig(t, content)
	defer os.Remove(path)

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "src.local", cfg.Source.Host)
	assert.Equal(t, 9000, cfg.Source.Port)
	assert.Equal(t, 50000, cfg.Sync.BatchSize)
	assert.Equal(t, 4, cfg.Sync.MaxConcurrency)
	assert.Equal(t, "info", cfg.Logging.Level)
	assert.True(t, cfg.Logging.Console)
	assert.Equal(t, "Asia/Kolkata", cfg.Timezone)
	assert.Len(t, cfg.Tables, 1)

	tbl, found := cfg.FindTable("table1")
	assert.True(t, found)
	assert.Equal(t, "date", tbl.DateColumn)

	_, found2 := cfg.FindTable("missing")
	assert.False(t, found2)

	loc, err := cfg.GetTimezone()
	require.NoError(t, err)
	assert.NotNil(t, loc)

	dur, err := cfg.GetStandbyInterval()
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), dur)

	dur2, err := cfg.GetRetryBackoff()
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), dur2)
}

func TestLoad_MissingSource(t *testing.T) {
	content := `
destination:
  host: "dest.local"
`
	path := writeTempConfig(t, content)
	defer os.Remove(path)

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source.host")
}

func TestLoad_InvalidPort(t *testing.T) {
	content := `
source:
  host: "src.local"
  port: 99999
destination:
  host: "dest.local"
`
	path := writeTempConfig(t, content)
	defer os.Remove(path)

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source.port")
}

func TestLoad_InvalidConcurrency(t *testing.T) {
	content := `
source:
  host: "src.local"
destination:
  host: "dest.local"
sync:
  max_concurrency: 100
`
	path := writeTempConfig(t, content)
	defer os.Remove(path)

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sync.max_concurrency")
}

func TestLoad_InvalidBatchSize(t *testing.T) {
	content := `
source:
  host: "src.local"
destination:
  host: "dest.local"
sync:
  batch_size: -1
`
	path := writeTempConfig(t, content)
	defer os.Remove(path)

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sync.batch_size")
}

func TestLoad_InvalidStandby(t *testing.T) {
	content := `
source:
  host: "src.local"
destination:
  host: "dest.local"
sync:
  standby:
    enabled: true
    interval: "-5m"
`
	path := writeTempConfig(t, content)
	defer os.Remove(path)

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sync.standby.interval")
}

func TestLoad_DuplicateTable(t *testing.T) {
	content := `
source:
  host: "src.local"
destination:
  host: "dest.local"
tables:
  - name: "table1"
    date_column: "date"
  - name: "table1"
    date_column: "date"
`
	path := writeTempConfig(t, content)
	defer os.Remove(path)

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate table entry")
}

func TestLoad_InvalidTableName(t *testing.T) {
	content := `
source:
  host: "src.local"
destination:
  host: "dest.local"
tables:
  - name: "invalid table!"
    date_column: "date"
`
	path := writeTempConfig(t, content)
	defer os.Remove(path)

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid table name format")
}

func TestLoad_MissingDateColumn(t *testing.T) {
	content := `
source:
  host: "src.local"
destination:
  host: "dest.local"
tables:
  - name: "table1"
`
	path := writeTempConfig(t, content)
	defer os.Remove(path)

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing date column")
}

func TestLoad_SameSourceDest(t *testing.T) {
	content := `
source:
  host: "src.local"
  port: 9000
  database: "default"
destination:
  host: "src.local"
  port: 9000
  database: "default"
`
	path := writeTempConfig(t, content)
	defer os.Remove(path)

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source and destination cannot be identical")
}

func TestLoad_InvalidTimezone(t *testing.T) {
	content := `
source:
  host: "src.local"
destination:
  host: "dest.local"
timezone: "Invalid/Zone"
`
	path := writeTempConfig(t, content)
	defer os.Remove(path)

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid timezone")
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	content := `
source:
  host: "src.local"
destination:
  host: "dest.local"
logging:
  level: "trace"
`
	path := writeTempConfig(t, content)
	defer os.Remove(path)

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid logging level")
}

func TestLoad_ExplicitNativeProtocol(t *testing.T) {
	content := `
source:
  host: "src.local"
  protocol: native
destination:
  host: "dest.local"
  protocol: native
tables:
  - name: "table1"
    date_column: "date"
`
	path := writeTempConfig(t, content)
	defer os.Remove(path)

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "native", cfg.Source.Protocol)
	assert.Equal(t, 9000, cfg.Source.Port)
	assert.Contains(t, cfg.Source.DSN(), "clickhouse://")
}

func TestLoad_HTTPProtocolDefaultPort(t *testing.T) {
	content := `
source:
  host: "src.local"
  protocol: http
destination:
  host: "dest.local"
  protocol: http
  port: 8124
tables:
  - name: "table1"
    date_column: "date"
`
	path := writeTempConfig(t, content)
	defer os.Remove(path)

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "http", cfg.Source.Protocol)
	// Port should have been defaulted to 8123 for http when not specified.
	assert.Equal(t, 8123, cfg.Source.Port)
	// Explicit port on destination must be respected.
	assert.Equal(t, 8124, cfg.Destination.Port)
	assert.Contains(t, cfg.Source.DSN(), "http://")
}

func TestLoad_InvalidProtocol(t *testing.T) {
	content := `
source:
  host: "src.local"
  protocol: grpc
destination:
  host: "dest.local"
`
	path := writeTempConfig(t, content)
	defer os.Remove(path)

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source.protocol")
}
