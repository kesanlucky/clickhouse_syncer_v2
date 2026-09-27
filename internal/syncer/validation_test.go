package syncer

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"clickhouse-syncer/internal/clickhouse"
)

func TestErrStr(t *testing.T) {
	assert.Equal(t, "", errStr(nil))
	assert.Equal(t, "some error", errStr(errors.New("some error")))
}

func TestErrStrNotExists(t *testing.T) {
	assert.Equal(t, "", errStrNotExists(nil, true))
	assert.Equal(t, "not found", errStrNotExists(nil, false))
	assert.Equal(t, "connection failed", errStrNotExists(errors.New("connection failed"), false))
}

func TestDateColErr(t *testing.T) {
	src := &clickhouse.ColumnInfo{Name: "date", Type: "Date"}
	dst := &clickhouse.ColumnInfo{Name: "date", Type: "Date"}

	assert.Equal(t, "", dateColErr(src, dst))
	assert.Equal(t, "missing on source", dateColErr(nil, dst))
	assert.Equal(t, "missing on destination", dateColErr(src, nil))
	assert.Equal(t, "missing on source", dateColErr(nil, nil))
}

func TestMatchErr(t *testing.T) {
	assert.Equal(t, "", matchErr("Date", "Date"))
	assert.Equal(t, "mismatch: source=Date, dest=DateTime", matchErr("Date", "DateTime"))
}

func TestSchemaDiffMsg(t *testing.T) {
	noDiff := &clickhouse.SchemaDiff{}
	assert.Equal(t, "", schemaDiffMsg(noDiff))

	missing := &clickhouse.SchemaDiff{MissingInDest: []string{"col1"}}
	assert.Equal(t, "missing in dest: [col1]", schemaDiffMsg(missing))

	extra := &clickhouse.SchemaDiff{ExtraInDest: []string{"col2"}}
	assert.Equal(t, "extra in dest: [col2]", schemaDiffMsg(extra))

	mismatch := &clickhouse.SchemaDiff{Mismatches: []clickhouse.ColumnMismatch{
		{Column: "col3", Field: "Type", Source: "Int32", Dest: "Int64"},
	}}
	assert.Equal(t, "mismatch col col3 field Type (src:Int32 dst:Int64)", schemaDiffMsg(mismatch))

	multiple := &clickhouse.SchemaDiff{
		MissingInDest: []string{"col1"},
		ExtraInDest:   []string{"col2"},
	}
	assert.Equal(t, "missing in dest: [col1]; extra in dest: [col2]", schemaDiffMsg(multiple))
}
