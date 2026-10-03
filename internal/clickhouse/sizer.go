package clickhouse

import (
	"strconv"
	"strings"
)

// ColumnSizer computes the uncompressed ClickHouse byte size of rows using
// column type metadata from DescribeTable. Fixed-width types are cached at
// construction time; variable-width types (String, Array, etc.) measure the
// actual value on each call.
//
// The result matches system.parts.data_uncompressed_bytes semantics: raw
// column bytes before compression, excluding block/index overhead.
type ColumnSizer struct {
	schema []ColumnInfo
	widths []int // per-column: >=0 fixed width, -1 variable-width
}

// NewColumnSizer creates a ColumnSizer from a table schema returned by DescribeTable.
func NewColumnSizer(schema []ColumnInfo) *ColumnSizer {
	widths := make([]int, len(schema))
	for i, col := range schema {
		widths[i] = chTypeWidth(col.Type)
	}
	return &ColumnSizer{schema: schema, widths: widths}
}

// BatchBytes returns the total uncompressed byte size for a batch of rows.
func (cs *ColumnSizer) BatchBytes(rows [][]any) uint64 {
	var total uint64
	for _, row := range rows {
		total += cs.RowBytes(row)
	}
	return total
}

// RowBytes returns the uncompressed byte size of a single row.
func (cs *ColumnSizer) RowBytes(row []any) uint64 {
	var total uint64
	n := len(cs.widths)
	if len(row) < n {
		n = len(row)
	}
	for i := 0; i < n; i++ {
		if cs.widths[i] >= 0 {
			total += uint64(cs.widths[i])
		} else {
			total += varBytes(row[i])
		}
	}
	return total
}

// chTypeWidth returns the fixed byte width for a ClickHouse type string, or -1
// for variable-width types (String, Array, Map, Tuple, etc.).
func chTypeWidth(typeName string) int {
	// Strip Nullable(T) → same width as T, +1 for the null byte
	if strings.HasPrefix(typeName, "Nullable(") && strings.HasSuffix(typeName, ")") {
		inner := typeName[len("Nullable(") : len(typeName)-1]
		w := chTypeWidth(inner)
		if w >= 0 {
			return w + 1
		}
		return -1
	}

	// Strip LowCardinality(T) → same width as T
	if strings.HasPrefix(typeName, "LowCardinality(") && strings.HasSuffix(typeName, ")") {
		inner := typeName[len("LowCardinality(") : len(typeName)-1]
		return chTypeWidth(inner)
	}

	// Strip SimpleAggregateFunction(fn, T) → same as T
	if strings.HasPrefix(typeName, "SimpleAggregateFunction(") && strings.HasSuffix(typeName, ")") {
		// Find the type after the first comma
		idx := strings.Index(typeName, ",")
		if idx > 0 {
			inner := strings.TrimSpace(typeName[idx+1 : len(typeName)-1])
			return chTypeWidth(inner)
		}
	}

	switch typeName {
	case "Bool", "UInt8", "Int8":
		return 1
	case "UInt16", "Int16", "Date":
		return 2
	case "UInt32", "Int32", "Float32", "Date32", "DateTime", "IPv4":
		return 4
	case "UInt64", "Int64", "Float64",
		"IntervalSecond", "IntervalMinute", "IntervalHour",
		"IntervalDay", "IntervalWeek", "IntervalMonth",
		"IntervalQuarter", "IntervalYear":
		return 8
	case "UInt128", "Int128", "UUID", "IPv6":
		return 16
	case "UInt256", "Int256":
		return 32
	}

	// DateTime64(precision) or DateTime64(precision, 'tz') → always 8 bytes
	if strings.HasPrefix(typeName, "DateTime64") {
		return 8
	}

	// FixedString(N) → N bytes
	if strings.HasPrefix(typeName, "FixedString(") && strings.HasSuffix(typeName, ")") {
		inner := typeName[len("FixedString(") : len(typeName)-1]
		if n, err := strconv.Atoi(strings.TrimSpace(inner)); err == nil {
			return n
		}
	}

	// Decimal32(S)/Decimal64(S)/Decimal128(S)/Decimal256(S) — short forms
	if strings.HasPrefix(typeName, "Decimal32(") {
		return 4
	}
	if strings.HasPrefix(typeName, "Decimal64(") {
		return 8
	}
	if strings.HasPrefix(typeName, "Decimal128(") {
		return 16
	}
	if strings.HasPrefix(typeName, "Decimal256(") {
		return 32
	}

	// Decimal(P, S) — full form: precision determines storage
	if strings.HasPrefix(typeName, "Decimal(") {
		p := decimalPrecision(typeName)
		switch {
		case p <= 9:
			return 4
		case p <= 18:
			return 8
		case p <= 38:
			return 16
		default:
			return 32
		}
	}

	// Enum8 / Enum16
	if strings.HasPrefix(typeName, "Enum8") {
		return 1
	}
	if strings.HasPrefix(typeName, "Enum16") {
		return 2
	}

	// Variable-width: String, Array, Map, Tuple, AggregateFunction, etc.
	return -1
}

// varBytes estimates the byte size of a variable-width value using its
// concrete type. For String/[]byte the length is exact; for other complex
// types (Array, Map, Tuple) we fall back to measuring the underlying string.
func varBytes(v any) uint64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case string:
		return uint64(len(val))
	case []byte:
		return uint64(len(val))
	case []any:
		// Array — recurse over elements (best-effort for mixed types)
		var total uint64
		for _, elem := range val {
			total += varBytes(elem)
		}
		return total
	default:
		// Map, Tuple, AggregateFunction, etc. — conservative 8-byte estimate
		return 8
	}
}

// decimalPrecision extracts the precision P from "Decimal(P, S)".
func decimalPrecision(typeName string) int {
	start := strings.Index(typeName, "(")
	end := strings.Index(typeName, ",")
	if start < 0 || end < 0 || end <= start+1 {
		return 9
	}
	p, err := strconv.Atoi(strings.TrimSpace(typeName[start+1 : end]))
	if err != nil {
		return 9
	}
	return p
}
