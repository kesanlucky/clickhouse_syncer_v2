package syncer

import (
	"context"
	"fmt"
	"strings"

	"clickhouse-syncer/internal/clickhouse"
	"clickhouse-syncer/internal/config"
)

type ValidationResult struct {
	Steps  []ValidationStep
	Passed bool
}

type ValidationStep struct {
	Name    string
	Passed  bool
	Message string
	Details string
}

func (s *Syncer) ValidateAll(ctx context.Context, tables []config.TableConfig) (*ValidationResult, error) {
	res := &ValidationResult{Passed: true}

	// 1. Config validation — must pass before we can use any config values below.
	err := s.config.Validate()
	res.Steps = append(res.Steps, ValidationStep{
		Name:    "Configuration Validation",
		Passed:  err == nil,
		Message: errStr(err),
	})
	if err != nil {
		res.Passed = false
		s.printValidationResult(res)
		return res, &SyncerError{Code: ExitConfigError, Message: "config validation failed", Err: err}
	}

	// 2. Source Connection
	err = s.source.Ping(ctx)
	res.Steps = append(res.Steps, ValidationStep{
		Name:    "Source Connection Ping",
		Passed:  err == nil,
		Message: errStr(err),
	})
	sourceUp := err == nil
	if err != nil {
		res.Passed = false
	}

	// 3. Dest Connection
	err = s.dest.Ping(ctx)
	res.Steps = append(res.Steps, ValidationStep{
		Name:    "Destination Connection Ping",
		Passed:  err == nil,
		Message: errStr(err),
	})
	destUp := err == nil
	if err != nil {
		res.Passed = false
	}

	// 4 & 5. Database exists — only checkable when connections are up.
	sourceDatabases := make(map[string]bool)
	for _, t := range tables {
		db := s.config.Source.Database
		parts := strings.Split(t.Name, ".")
		if len(parts) == 2 {
			db = parts[0]
		}
		sourceDatabases[db] = true
	}

	sourceDbsOk := true
	if sourceUp {
		for db := range sourceDatabases {
			exists, err := s.source.DatabaseExists(ctx, db)
			res.Steps = append(res.Steps, ValidationStep{
				Name:    fmt.Sprintf("Source Database Exists: %s", db),
				Passed:  err == nil && exists,
				Message: errStrNotExists(err, exists),
			})
			if err != nil || !exists {
				res.Passed = false
				sourceDbsOk = false
			}
		}
	}

	destDbOk := true
	if destUp {
		destDb := s.config.Destination.Database
		exists, err := s.dest.DatabaseExists(ctx, destDb)
		res.Steps = append(res.Steps, ValidationStep{
			Name:    fmt.Sprintf("Destination Database Exists: %s", destDb),
			Passed:  err == nil && exists,
			Message: errStrNotExists(err, exists),
		})
		if err != nil || !exists {
			res.Passed = false
			destDbOk = false
		}
	}

	// 6-10. Table specific validations — only when both DBs are reachable and exist.
	if sourceUp && destUp && sourceDbsOk && destDbOk {
		for _, t := range tables {
			// Source table exists
			srcExists, err := s.source.TableExists(ctx, t.Name)
			res.Steps = append(res.Steps, ValidationStep{
				Name:    fmt.Sprintf("Table Source Exists: %s", t.Name),
				Passed:  err == nil && srcExists,
				Message: errStrNotExists(err, srcExists),
			})
			if err != nil || !srcExists {
				res.Passed = false
				continue
			}

			// Dest table exists
			dstExists, err := s.dest.TableExists(ctx, t.Name)
			res.Steps = append(res.Steps, ValidationStep{
				Name:    fmt.Sprintf("Table Destination Exists: %s", t.Name),
				Passed:  err == nil && dstExists,
				Message: errStrNotExists(err, dstExists),
			})
			if err != nil || !dstExists {
				res.Passed = false
				continue
			}

			// Describe schemas
			srcSchema, err := s.source.DescribeTable(ctx, t.Name)
			if err != nil {
				res.Passed = false
				res.Steps = append(res.Steps, ValidationStep{Name: "Source Describe Table: " + t.Name, Passed: false, Message: err.Error()})
				continue
			}

			dstSchema, err := s.dest.DescribeTable(ctx, t.Name)
			if err != nil {
				res.Passed = false
				res.Steps = append(res.Steps, ValidationStep{Name: "Destination Describe Table: " + t.Name, Passed: false, Message: err.Error()})
				continue
			}

			// Date column check
			var srcDateCol *clickhouse.ColumnInfo
			for _, col := range srcSchema {
				if col.Name == t.DateColumn {
					srcDateCol = &col
					break
				}
			}

			var dstDateCol *clickhouse.ColumnInfo
			for _, col := range dstSchema {
				if col.Name == t.DateColumn {
					dstDateCol = &col
					break
				}
			}

			res.Steps = append(res.Steps, ValidationStep{
				Name:    fmt.Sprintf("Date Column Exists: %s.%s", t.Name, t.DateColumn),
				Passed:  srcDateCol != nil && dstDateCol != nil,
				Message: dateColErr(srcDateCol, dstDateCol),
			})
			if srcDateCol == nil || dstDateCol == nil {
				res.Passed = false
				continue
			}

			_, srcErr := clickhouse.ParseDateColumnType(srcDateCol.Type)
			_, dstErr := clickhouse.ParseDateColumnType(dstDateCol.Type)

			res.Steps = append(res.Steps, ValidationStep{
				Name:    fmt.Sprintf("Date Column Supported: %s.%s", t.Name, t.DateColumn),
				Passed:  srcErr == nil && dstErr == nil,
				Message: typeErr(srcErr, dstErr),
			})
			if srcErr != nil || dstErr != nil {
				res.Passed = false
				continue
			}

			res.Steps = append(res.Steps, ValidationStep{
				Name:    fmt.Sprintf("Date Column Match: %s.%s", t.Name, t.DateColumn),
				Passed:  srcDateCol.Type == dstDateCol.Type,
				Message: matchErr(srcDateCol.Type, dstDateCol.Type),
			})
			if srcDateCol.Type != dstDateCol.Type {
				res.Passed = false
				continue
			}

			// Schema diff
			diff := clickhouse.CompareSchemas(srcSchema, dstSchema)
			res.Steps = append(res.Steps, ValidationStep{
				Name:    fmt.Sprintf("Schema Comparison: %s", t.Name),
				Passed:  !diff.HasDifferences(),
				Message: schemaDiffMsg(diff),
			})
			if diff.HasDifferences() {
				res.Passed = false
			}
		}
	}

	s.printValidationResult(res)
	if !res.Passed {
		return res, &SyncerError{Code: ExitValidationError, Message: "validation failed"}
	}
	return res, nil
}

func (s *Syncer) ValidateForDelete(ctx context.Context, tables []config.TableConfig) (*ValidationResult, error) {
	res := &ValidationResult{Passed: true}

	// 1. Config validation — must pass before we can use any config values below.
	err := s.config.Validate()
	res.Steps = append(res.Steps, ValidationStep{Name: "Configuration Validation", Passed: err == nil, Message: errStr(err)})
	if err != nil {
		res.Passed = false
		s.printValidationResult(res)
		return res, &SyncerError{Code: ExitConfigError, Message: "config validation failed", Err: err}
	}

	// 2. Dest Connection
	err = s.dest.Ping(ctx)
	res.Steps = append(res.Steps, ValidationStep{Name: "Destination Connection Ping", Passed: err == nil, Message: errStr(err)})
	destUp := err == nil
	if err != nil {
		res.Passed = false
	}

	// 3. Dest DB exists
	destDbOk := false
	if destUp {
		destDb := s.config.Destination.Database
		exists, err := s.dest.DatabaseExists(ctx, destDb)
		res.Steps = append(res.Steps, ValidationStep{
			Name:    fmt.Sprintf("Destination Database Exists: %s", destDb),
			Passed:  err == nil && exists,
			Message: errStrNotExists(err, exists),
		})
		if err != nil || !exists {
			res.Passed = false
		} else {
			destDbOk = true
		}
	}

	// 4+. Table specific validations
	if destUp && destDbOk {
		for _, t := range tables {
			dstExists, err := s.dest.TableExists(ctx, t.Name)
			res.Steps = append(res.Steps, ValidationStep{
				Name:    fmt.Sprintf("Table Destination Exists: %s", t.Name),
				Passed:  err == nil && dstExists,
				Message: errStrNotExists(err, dstExists),
			})
			if err != nil || !dstExists {
				res.Passed = false
				continue
			}

			dstSchema, err := s.dest.DescribeTable(ctx, t.Name)
			if err != nil {
				res.Passed = false
				res.Steps = append(res.Steps, ValidationStep{Name: "Destination Describe Table: " + t.Name, Passed: false, Message: err.Error()})
				continue
			}

			var dstDateCol *clickhouse.ColumnInfo
			for _, col := range dstSchema {
				if col.Name == t.DateColumn {
					dstDateCol = &col
					break
				}
			}

			res.Steps = append(res.Steps, ValidationStep{
				Name:    fmt.Sprintf("Date Column Exists: %s.%s", t.Name, t.DateColumn),
				Passed:  dstDateCol != nil,
				Message: dateColErrDest(dstDateCol),
			})
			if dstDateCol == nil {
				res.Passed = false
				continue
			}

			_, dstErr := clickhouse.ParseDateColumnType(dstDateCol.Type)
			res.Steps = append(res.Steps, ValidationStep{
				Name:    fmt.Sprintf("Date Column Supported: %s.%s", t.Name, t.DateColumn),
				Passed:  dstErr == nil,
				Message: typeErrDest(dstErr),
			})
			if dstErr != nil {
				res.Passed = false
			}
		}
	}

	s.printValidationResult(res)
	if !res.Passed {
		return res, &SyncerError{Code: ExitValidationError, Message: "validation failed"}
	}
	return res, nil
}

// Helpers
func errStr(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}
func errStrNotExists(err error, exists bool) string {
	if err != nil {
		return err.Error()
	}
	if !exists {
		return "not found"
	}
	return ""
}
func dateColErr(src, dst *clickhouse.ColumnInfo) string {
	if src == nil {
		return "missing on source"
	}
	if dst == nil {
		return "missing on destination"
	}
	return ""
}
func dateColErrDest(dst *clickhouse.ColumnInfo) string {
	if dst == nil {
		return "missing on destination"
	}
	return ""
}
func typeErr(src, dst error) string {
	if src != nil {
		return "source: " + src.Error()
	}
	if dst != nil {
		return "dest: " + dst.Error()
	}
	return ""
}
func typeErrDest(dst error) string {
	if dst != nil {
		return "dest: " + dst.Error()
	}
	return ""
}
func matchErr(src, dst string) string {
	if src != dst {
		return fmt.Sprintf("mismatch: source=%s, dest=%s", src, dst)
	}
	return ""
}
func schemaDiffMsg(diff *clickhouse.SchemaDiff) string {
	if !diff.HasDifferences() {
		return ""
	}
	var msgs []string
	if len(diff.MissingInDest) > 0 {
		msgs = append(msgs, fmt.Sprintf("missing in dest: %v", diff.MissingInDest))
	}
	for _, m := range diff.Mismatches {
		msgs = append(msgs, fmt.Sprintf("mismatch col %s field %s (src:%s dst:%s)", m.Column, m.Field, m.Source, m.Dest))
	}
	return strings.Join(msgs, "; ")
}
