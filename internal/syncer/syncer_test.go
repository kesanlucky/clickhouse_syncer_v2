package syncer

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"clickhouse-syncer/internal/config"
)

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		input    uint64
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{100, "100"},
		{1000, "1,000"},
		{10000, "10,000"},
		{100000, "100,000"},
		{1000000, "1,000,000"},
		{1248210, "1,248,210"},
		{18432991, "18,432,991"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, formatNumber(tt.input))
		})
	}
}

func TestOperationString(t *testing.T) {
	assert.Equal(t, "SYNC", OpSync.String())
	assert.Equal(t, "DELETE", OpDelete.String())
	assert.Equal(t, "DRY RUN", OpDryRun.String())
	assert.Equal(t, "VALIDATE", OpValidate.String())
	assert.Equal(t, "STANDBY SYNC", OpStandbySync.String())
	assert.Equal(t, "UNKNOWN", Operation(99).String())
}

func TestExitCodes(t *testing.T) {
	assert.Equal(t, ExitCode(0), ExitSuccess)
	assert.Equal(t, ExitCode(1), ExitGeneralFailure)
	assert.Equal(t, ExitCode(2), ExitConfigError)
	assert.Equal(t, ExitCode(3), ExitValidationError)
	assert.Equal(t, ExitCode(6), ExitSyncFailure)
	assert.Equal(t, ExitCode(8), ExitDeleteFailure)
}

func TestSyncerError(t *testing.T) {
	innerErr := errors.New("inner failure")
	
	errWithInner := &SyncerError{Code: ExitGeneralFailure, Message: "operation failed", Err: innerErr}
	assert.Equal(t, "operation failed: inner failure", errWithInner.Error())
	assert.Equal(t, innerErr, errWithInner.Unwrap())

	errWithoutInner := &SyncerError{Code: ExitConfigError, Message: "config missing"}
	assert.Equal(t, "config missing", errWithoutInner.Error())
	assert.Nil(t, errWithoutInner.Unwrap())
}

func newTestSyncer(t *testing.T) *Syncer {
	tz, err := time.LoadLocation("Asia/Kolkata")
	require.NoError(t, err)

	return &Syncer{
		config: &config.Config{
			Tables: []config.TableConfig{
				{Name: "db1.table1", DateColumn: "created_at"},
				{Name: "db1.table2", DateColumn: "updated_at"},
			},
		},
		timezone: tz,
	}
}

func TestParseDate(t *testing.T) {
	s := newTestSyncer(t)

	// Valid date
	dt, err := s.parseDate("2023-10-25")
	require.NoError(t, err)
	assert.Equal(t, 2023, dt.Year())
	assert.Equal(t, time.October, dt.Month())
	assert.Equal(t, 25, dt.Day())
	assert.Equal(t, "Asia/Kolkata", dt.Location().String())

	// Invalid format
	_, err = s.parseDate("2023/10/25")
	assert.Error(t, err)
}

func TestResolveTables(t *testing.T) {
	s := newTestSyncer(t)

	// Empty input -> return all
	all := s.resolveTables(nil)
	assert.Len(t, all, 2)
	assert.Equal(t, "db1.table1", all[0].Name)

	emptyList := []config.TableConfig{}
	allAgain := s.resolveTables(emptyList)
	assert.Len(t, allAgain, 2)

	// Non-empty input -> return selection
	selected := []config.TableConfig{
		{Name: "db1.table2", DateColumn: "updated_at"},
	}
	subset := s.resolveTables(selected)
	assert.Len(t, subset, 1)
	assert.Equal(t, "db1.table2", subset[0].Name)
}
