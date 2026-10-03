package clickhouse

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

type DateColumnType int

const (
	DateTypeDate DateColumnType = iota
	DateTypeDate32
	DateTypeDateTime
	DateTypeDateTime64
)

func (t DateColumnType) String() string {
	switch t {
	case DateTypeDate:
		return "Date"
	case DateTypeDate32:
		return "Date32"
	case DateTypeDateTime:
		return "DateTime"
	case DateTypeDateTime64:
		return "DateTime64"
	default:
		return "Unknown"
	}
}

type ColumnInfo struct {
	Name              string
	Type              string
	Position          uint64
	DefaultKind       string
	DefaultExpression string
	Comment           string
	CodecExpression   string
	IsInPrimaryKey    bool
	IsInSortingKey    bool
	IsInPartitionKey  bool
}

type TableMeta struct {
	Database     string
	Table        string
	Engine       string
	PartitionKey string
	SortingKey   string
	PrimaryKey   string
	SamplingKey  string
}

type SchemaDiff struct {
	Table         string
	Mismatches    []ColumnMismatch
	MissingInDest []string
	ExtraInDest   []string
}

func (d *SchemaDiff) HasDifferences() bool {
	return len(d.Mismatches) > 0 || len(d.MissingInDest) > 0
}

func (d *SchemaDiff) String() string {
	var parts []string
	if len(d.MissingInDest) > 0 {
		parts = append(parts, fmt.Sprintf("Missing in dest: %s", strings.Join(d.MissingInDest, ", ")))
	}
	if len(d.ExtraInDest) > 0 {
		parts = append(parts, fmt.Sprintf("Extra in dest: %s", strings.Join(d.ExtraInDest, ", ")))
	}
	for _, m := range d.Mismatches {
		parts = append(parts, fmt.Sprintf("Mismatch %s: %s (src: %s, dest: %s)", m.Column, m.Field, m.Source, m.Dest))
	}
	if len(parts) == 0 {
		return "No differences"
	}
	return strings.Join(parts, "; ")
}

type ColumnMismatch struct {
	Column string
	Field  string
	Source string
	Dest   string
}

var identifierRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
var tableNameRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*\.[a-zA-Z_][a-zA-Z0-9_]*$`)

func QuoteIdentifier(name string) string {
	escaped := strings.ReplaceAll(name, "`", "\\`")
	return fmt.Sprintf("`%s`", escaped)
}

func QuoteTableName(fullName string) (string, error) {
	parts := strings.Split(fullName, ".")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid table name format, expected 'database.table': %s", fullName)
	}
	return fmt.Sprintf("%s.%s", QuoteIdentifier(parts[0]), QuoteIdentifier(parts[1])), nil
}

func ValidateIdentifier(name string) error {
	if !identifierRegex.MatchString(name) {
		return fmt.Errorf("invalid identifier: %s", name)
	}
	return nil
}

func ValidateTableName(fullName string) error {
	if !tableNameRegex.MatchString(fullName) {
		return fmt.Errorf("invalid table name format: %s", fullName)
	}
	return nil
}

func SplitTableName(fullName string) (database, table string, err error) {
	parts := strings.Split(fullName, ".")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid table name format: %s", fullName)
	}
	return parts[0], parts[1], nil
}

func ParseDateColumnType(typeName string) (DateColumnType, error) {
	if strings.HasPrefix(typeName, "Date32") {
		return DateTypeDate32, nil
	}
	if strings.HasPrefix(typeName, "Date") && !strings.HasPrefix(typeName, "DateTime") {
		return DateTypeDate, nil
	}
	if strings.HasPrefix(typeName, "DateTime64") {
		return DateTypeDateTime64, nil
	}
	if strings.HasPrefix(typeName, "DateTime") {
		return DateTypeDateTime, nil
	}
	return 0, fmt.Errorf("unsupported date column type: %s", typeName)
}

func FormatDateBoundary(t time.Time, dateType DateColumnType) string {
	switch dateType {
	case DateTypeDate, DateTypeDate32:
		return t.Format("2006-01-02")
	case DateTypeDateTime:
		return t.Format("2006-01-02 15:04:05")
	case DateTypeDateTime64:
		return t.Format("2006-01-02 15:04:05.000000")
	default:
		return t.Format(time.RFC3339)
	}
}

func FormatDateBoundaryEnd(t time.Time, dateType DateColumnType) string {
	switch dateType {
	case DateTypeDate, DateTypeDate32:
		return t.AddDate(0, 0, 1).Format("2006-01-02")
	case DateTypeDateTime:
		next := t.AddDate(0, 0, 1)
		next = time.Date(next.Year(), next.Month(), next.Day(), 0, 0, 0, 0, t.Location())
		return next.Format("2006-01-02 15:04:05")
	case DateTypeDateTime64:
		next := t.AddDate(0, 0, 1)
		next = time.Date(next.Year(), next.Month(), next.Day(), 0, 0, 0, 0, t.Location())
		return next.Format("2006-01-02 15:04:05.000000")
	default:
		return t.Format(time.RFC3339)
	}
}

func CompareSchemas(source, dest []ColumnInfo) *SchemaDiff {
	diff := &SchemaDiff{}

	srcMap := make(map[string]ColumnInfo)
	for _, c := range source {
		srcMap[c.Name] = c
	}

	destMap := make(map[string]ColumnInfo)
	for _, c := range dest {
		destMap[c.Name] = c
	}

	// Iterate over source columns in order to check for missing and mismatched columns
	for _, srcCol := range source {
		destCol, exists := destMap[srcCol.Name]
		if !exists {
			diff.MissingInDest = append(diff.MissingInDest, srcCol.Name)
			continue
		}

		if srcCol.Type != destCol.Type {
			diff.Mismatches = append(diff.Mismatches, ColumnMismatch{
				Column: srcCol.Name, Field: "type",
				Source: srcCol.Type, Dest: destCol.Type,
			})
		}

		if srcCol.DefaultKind != destCol.DefaultKind {
			diff.Mismatches = append(diff.Mismatches, ColumnMismatch{
				Column: srcCol.Name, Field: "default_kind",
				Source: srcCol.DefaultKind, Dest: destCol.DefaultKind,
			})
		}

		if srcCol.DefaultExpression != destCol.DefaultExpression {
			diff.Mismatches = append(diff.Mismatches, ColumnMismatch{
				Column: srcCol.Name, Field: "default_expression",
				Source: srcCol.DefaultExpression, Dest: destCol.DefaultExpression,
			})
		}

		if srcCol.CodecExpression != destCol.CodecExpression {
			diff.Mismatches = append(diff.Mismatches, ColumnMismatch{
				Column: srcCol.Name, Field: "codec_expression",
				Source: srcCol.CodecExpression, Dest: destCol.CodecExpression,
			})
		}
	}

	// Check for extra columns in destination
	for name := range destMap {
		if _, exists := srcMap[name]; !exists {
			diff.ExtraInDest = append(diff.ExtraInDest, name)
		}
	}

	return diff
}

func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "Code: 210") ||
		strings.Contains(msg, "Code: 279") ||
		strings.Contains(msg, "Code: 242") {
		return true
	}
	return false
}
