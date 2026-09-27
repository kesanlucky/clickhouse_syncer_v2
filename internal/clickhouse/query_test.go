package clickhouse

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuoteIdentifier(t *testing.T) {
	assert.Equal(t, "`col`", QuoteIdentifier("col"))
	assert.Equal(t, "`col\\`name`", QuoteIdentifier("col`name"))
	assert.Equal(t, "``", QuoteIdentifier(""))
}

func TestQuoteTableName(t *testing.T) {
	val, err := QuoteTableName("db.table")
	require.NoError(t, err)
	assert.Equal(t, "`db`.`table`", val)

	_, err = QuoteTableName("table")
	require.Error(t, err)
}

func TestValidateIdentifier(t *testing.T) {
	assert.NoError(t, ValidateIdentifier("valid_col"))
	assert.Error(t, ValidateIdentifier("invalid col"))
	assert.Error(t, ValidateIdentifier("1col"))
}

func TestValidateTableName(t *testing.T) {
	assert.NoError(t, ValidateTableName("db.table"))
	assert.Error(t, ValidateTableName("table"))
	assert.Error(t, ValidateTableName("db.table.extra"))
}

func TestParseDateColumnType(t *testing.T) {
	dt, err := ParseDateColumnType("Date")
	require.NoError(t, err)
	assert.Equal(t, DateTypeDate, dt)

	dt, err = ParseDateColumnType("Date32")
	require.NoError(t, err)
	assert.Equal(t, DateTypeDate32, dt)

	dt, err = ParseDateColumnType("DateTime('Asia/Kolkata')")
	require.NoError(t, err)
	assert.Equal(t, DateTypeDateTime, dt)

	dt, err = ParseDateColumnType("DateTime64(3)")
	require.NoError(t, err)
	assert.Equal(t, DateTypeDateTime64, dt)

	_, err = ParseDateColumnType("String")
	require.Error(t, err)
}

func TestFormatDateBoundary(t *testing.T) {
	tm := time.Date(2023, 10, 25, 14, 30, 0, 0, time.UTC)
	assert.Equal(t, "2023-10-25", FormatDateBoundary(tm, DateTypeDate))
	assert.Equal(t, "2023-10-25", FormatDateBoundary(tm, DateTypeDate32))
	assert.Equal(t, "2023-10-25 14:30:00", FormatDateBoundary(tm, DateTypeDateTime))
	assert.Equal(t, "2023-10-25 14:30:00.000000", FormatDateBoundary(tm, DateTypeDateTime64))
}

func TestFormatDateBoundaryEnd(t *testing.T) {
	tm := time.Date(2023, 10, 25, 14, 30, 0, 0, time.UTC)
	assert.Equal(t, "2023-10-26", FormatDateBoundaryEnd(tm, DateTypeDate))
	assert.Equal(t, "2023-10-26 00:00:00", FormatDateBoundaryEnd(tm, DateTypeDateTime))
	assert.Equal(t, "2023-10-26 00:00:00.000000", FormatDateBoundaryEnd(tm, DateTypeDateTime64))
}

func TestCompareSchemas(t *testing.T) {
	src := []ColumnInfo{
		{Name: "id", Type: "UInt64"},
		{Name: "name", Type: "String"},
	}
	dest := []ColumnInfo{
		{Name: "id", Type: "UInt64"},
		{Name: "name", Type: "String"},
	}

	diff := CompareSchemas(src, dest)
	assert.False(t, diff.HasDifferences())

	dest[1].Type = "Int32"
	diff = CompareSchemas(src, dest)
	assert.True(t, diff.HasDifferences())
	assert.Len(t, diff.Mismatches, 1)

	dest = append(dest, ColumnInfo{Name: "extra", Type: "String"})
	diff = CompareSchemas(src, dest)
	assert.Len(t, diff.ExtraInDest, 1)

	src = append(src, ColumnInfo{Name: "missing", Type: "String"})
	diff = CompareSchemas(src, dest)
	assert.Len(t, diff.MissingInDest, 1)
}

func TestSplitTableName(t *testing.T) {
	db, tbl, err := SplitTableName("db.table")
	require.NoError(t, err)
	assert.Equal(t, "db", db)
	assert.Equal(t, "table", tbl)

	_, _, err = SplitTableName("table")
	require.Error(t, err)
}

func TestIsRetryableError(t *testing.T) {
	assert.True(t, IsRetryableError(errors.New("dial tcp: connection refused")))
	assert.True(t, IsRetryableError(errors.New("Exception: Code: 210")))
	assert.False(t, IsRetryableError(errors.New("Code: 44. Syntax error")))
	assert.False(t, IsRetryableError(nil))
}

func TestDateColumnType_String(t *testing.T) {
	assert.Equal(t, "Date", DateTypeDate.String())
	assert.Equal(t, "Date32", DateTypeDate32.String())
	assert.Equal(t, "DateTime", DateTypeDateTime.String())
	assert.Equal(t, "DateTime64", DateTypeDateTime64.String())
	assert.Equal(t, "Unknown", DateColumnType(99).String())
}

func TestSchemaDiff_String(t *testing.T) {
	diff := &SchemaDiff{
		MissingInDest: []string{"col1"},
		ExtraInDest:   []string{"col2"},
		Mismatches: []ColumnMismatch{
			{Column: "col3", Field: "Type", Source: "String", Dest: "Int32"},
		},
	}
	str := diff.String()
	assert.Contains(t, str, "Missing in dest: col1")
	assert.Contains(t, str, "Extra in dest: col2")
	assert.Contains(t, str, "Mismatch col3: Type")
}
