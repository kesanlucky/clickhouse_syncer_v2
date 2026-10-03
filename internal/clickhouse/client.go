package clickhouse

import (
	"context"
	"fmt"
	"time"

	"clickhouse-syncer/internal/config"
	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// Client represents a connection to a ClickHouse database
type Client struct {
	conn     driver.Conn
	host     string
	port     int
	database string
	role     string // "source" or "destination"
}

// NewClient creates a new ClickHouse client using the configured protocol (native or http)
func NewClient(ctx context.Context, cfg config.ClickHouseConfig, role string) (*Client, error) {
	// Map config protocol string to clickhouse-go's Protocol type.
	proto := clickhouse.Native
	if cfg.Protocol == "http" {
		proto = clickhouse.HTTP
	}

	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr:     []string{fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)},
		Protocol: proto,
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.User,
			Password: cfg.Password,
		},
		ClientInfo: clickhouse.ClientInfo{
			Products: []struct {
				Name    string
				Version string
			}{
				{Name: "clickhouse-syncer", Version: "1.0.0"},
			},
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		DialTimeout:     10 * time.Second,
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: 10 * time.Minute,
		ReadTimeout:     5 * time.Minute,
		BlockBufferSize: 10,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open connection: %w", err)
	}

	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping clickhouse: %w", err)
	}

	return &Client{
		conn:     conn,
		host:     cfg.Host,
		port:     cfg.Port,
		database: cfg.Database,
		role:     role,
	}, nil
}

// Ping checks the connection to ClickHouse
func (c *Client) Ping(ctx context.Context) error {
	return c.conn.Ping(ctx)
}

// Close closes the underlying database connection
func (c *Client) Close() error {
	return c.conn.Close()
}

// Role returns the role of this client (e.g., source or destination)
func (c *Client) Role() string {
	return c.role
}

// Addr returns the host:port string for the client
func (c *Client) Addr() string {
	return fmt.Sprintf("%s:%d", c.host, c.port)
}

// DatabaseExists checks if a database exists in the system
func (c *Client) DatabaseExists(ctx context.Context, database string) (bool, error) {
	query := "SELECT count() FROM system.databases WHERE name = $1"
	var count uint64
	err := c.conn.QueryRow(ctx, query, database).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check if database exists: %w", err)
	}
	return count > 0, nil
}

// TableExists checks if a table exists in the system
func (c *Client) TableExists(ctx context.Context, fullTableName string) (bool, error) {
	db, table, err := SplitTableName(fullTableName)
	if err != nil {
		return false, err
	}
	if db == "" {
		db = c.database
	}

	query := "SELECT count() FROM system.tables WHERE database = $1 AND name = $2"
	var count uint64
	err = c.conn.QueryRow(ctx, query, db, table).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check if table exists: %w", err)
	}
	return count > 0, nil
}

// DescribeTable retrieves schema information for all columns in a table
func (c *Client) DescribeTable(ctx context.Context, fullTableName string) ([]ColumnInfo, error) {
	db, table, err := SplitTableName(fullTableName)
	if err != nil {
		return nil, err
	}
	if db == "" {
		db = c.database
	}

	query := `
		SELECT 
			name, 
			type, 
			position, 
			default_kind, 
			default_expression, 
			comment, 
			compression_codec AS codec_expression, 
			is_in_primary_key, 
			is_in_sorting_key, 
			is_in_partition_key
		FROM system.columns
		WHERE database = $1 AND table = $2
		ORDER BY position ASC
	`
	rows, err := c.conn.Query(ctx, query, db, table)
	if err != nil {
		return nil, fmt.Errorf("failed to query columns: %w", err)
	}
	defer rows.Close()

	var cols []ColumnInfo
	for rows.Next() {
		var info ColumnInfo
		var defaultKind, defaultExpr, comment, codecExpr string
		var isInPrimaryKey, isInSortingKey, isInPartitionKey uint8

		err := rows.Scan(
			&info.Name,
			&info.Type,
			&info.Position,
			&defaultKind,
			&defaultExpr,
			&comment,
			&codecExpr,
			&isInPrimaryKey,
			&isInSortingKey,
			&isInPartitionKey,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan column info: %w", err)
		}

		info.DefaultKind = defaultKind
		info.DefaultExpression = defaultExpr
		info.Comment = comment
		info.CodecExpression = codecExpr
		info.IsInPrimaryKey = isInPrimaryKey > 0
		info.IsInSortingKey = isInSortingKey > 0
		info.IsInPartitionKey = isInPartitionKey > 0

		cols = append(cols, info)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return cols, nil
}

// GetTableMetadata retrieves engine and key information for a table
func (c *Client) GetTableMetadata(ctx context.Context, fullTableName string) (*TableMeta, error) {
	db, table, err := SplitTableName(fullTableName)
	if err != nil {
		return nil, err
	}
	if db == "" {
		db = c.database
	}

	query := `
		SELECT 
			engine,
			partition_key,
			sorting_key,
			primary_key,
			sampling_key
		FROM system.tables
		WHERE database = $1 AND name = $2
	`
	var meta TableMeta
	meta.Database = db
	meta.Table = table

	err = c.conn.QueryRow(ctx, query, db, table).Scan(
		&meta.Engine,
		&meta.PartitionKey,
		&meta.SortingKey,
		&meta.PrimaryKey,
		&meta.SamplingKey,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get table metadata: %w", err)
	}
	return &meta, nil
}

// GetDateColumnType retrieves and parses the date type of a specified column
func (c *Client) GetDateColumnType(ctx context.Context, fullTableName string, dateColumn string) (DateColumnType, error) {
	db, table, err := SplitTableName(fullTableName)
	if err != nil {
		return 0, err
	}
	if db == "" {
		db = c.database
	}

	query := "SELECT type FROM system.columns WHERE database = $1 AND table = $2 AND name = $3"
	var colType string
	err = c.conn.QueryRow(ctx, query, db, table, dateColumn).Scan(&colType)
	if err != nil {
		return 0, fmt.Errorf("failed to get column type: %w", err)
	}

	return ParseDateColumnType(colType)
}

// CountRows counts the number of rows in a given date range (half-open interval)
func (c *Client) CountRows(ctx context.Context, table string, dateCol string, start time.Time, end time.Time, dateType DateColumnType) (uint64, error) {
	quotedTable, err := QuoteTableName(table)
	if err != nil {
		return 0, err
	}

	quotedDateCol := QuoteIdentifier(dateCol)
	startStr := FormatDateBoundary(start, dateType)
	endStr := FormatDateBoundary(end, dateType)

	query := fmt.Sprintf("SELECT count() FROM %s WHERE %s >= '%s' AND %s < '%s'", quotedTable, quotedDateCol, startStr, quotedDateCol, endStr)

	var count uint64
	err = c.conn.QueryRow(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count rows: %w", err)
	}
	return count, nil
}

// CountRowsBefore counts the number of rows before a given cutoff time
func (c *Client) CountRowsBefore(ctx context.Context, table string, dateCol string, cutoff time.Time, dateType DateColumnType) (uint64, error) {
	quotedTable, err := QuoteTableName(table)
	if err != nil {
		return 0, err
	}

	quotedDateCol := QuoteIdentifier(dateCol)
	cutoffStr := FormatDateBoundary(cutoff, dateType)

	query := fmt.Sprintf("SELECT count() FROM %s WHERE %s < '%s'", quotedTable, quotedDateCol, cutoffStr)

	var count uint64
	err = c.conn.QueryRow(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count rows before: %w", err)
	}
	return count, nil
}

// CountAllRows counts all rows in a table
func (c *Client) CountAllRows(ctx context.Context, table string) (uint64, error) {
	quotedTable, err := QuoteTableName(table)
	if err != nil {
		return 0, err
	}

	query := fmt.Sprintf("SELECT count() FROM %s", quotedTable)
	var count uint64
	err = c.conn.QueryRow(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count all rows: %w", err)
	}
	return count, nil
}

// DeleteDateRange deletes rows in a half-open date interval
func (c *Client) DeleteDateRange(ctx context.Context, table string, dateCol string, start time.Time, end time.Time, dateType DateColumnType) error {
	quotedTable, err := QuoteTableName(table)
	if err != nil {
		return err
	}

	quotedDateCol := QuoteIdentifier(dateCol)
	startStr := FormatDateBoundary(start, dateType)
	endStr := FormatDateBoundary(end, dateType)

	query := fmt.Sprintf("ALTER TABLE %s DELETE WHERE %s >= '%s' AND %s < '%s'", quotedTable, quotedDateCol, startStr, quotedDateCol, endStr)

	err = c.conn.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to delete date range: %w", err)
	}
	return nil
}

// DeleteBefore deletes rows before a cutoff time
func (c *Client) DeleteBefore(ctx context.Context, table string, dateCol string, cutoff time.Time, dateType DateColumnType) error {
	quotedTable, err := QuoteTableName(table)
	if err != nil {
		return err
	}

	quotedDateCol := QuoteIdentifier(dateCol)
	cutoffStr := FormatDateBoundary(cutoff, dateType)

	query := fmt.Sprintf("ALTER TABLE %s DELETE WHERE %s < '%s'", quotedTable, quotedDateCol, cutoffStr)

	err = c.conn.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to delete before: %w", err)
	}
	return nil
}

// WaitForMutations blocks until all mutations on a table are complete, or the timeout is reached
func (c *Client) WaitForMutations(ctx context.Context, fullTableName string, timeout time.Duration) error {
	db, table, err := SplitTableName(fullTableName)
	if err != nil {
		return err
	}
	if db == "" {
		db = c.database
	}

	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	backoff := 100 * time.Millisecond
	maxBackoff := 5 * time.Second

	for {
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("timeout waiting for mutations to complete on table %s.%s", db, table)
		default:
		}

		query := "SELECT count() FROM system.mutations WHERE database = $1 AND table = $2 AND is_done = 0"
		var count uint64
		err := c.conn.QueryRow(timeoutCtx, query, db, table).Scan(&count)
		if err != nil {
			return fmt.Errorf("failed to check mutations: %w", err)
		}

		if count == 0 {
			return nil
		}

		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("timeout waiting for mutations to complete on table %s.%s", db, table)
		case <-time.After(backoff):
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

// GetMinMaxDate retrieves the minimum and maximum dates within a specified range
func (c *Client) GetMinMaxDate(ctx context.Context, table string, dateCol string, start time.Time, end time.Time, dateType DateColumnType) (min, max time.Time, err error) {
	quotedTable, err := QuoteTableName(table)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	quotedDateCol := QuoteIdentifier(dateCol)
	startStr := FormatDateBoundary(start, dateType)
	endStr := FormatDateBoundary(end, dateType)

	query := fmt.Sprintf("SELECT min(%s), max(%s) FROM %s WHERE %s >= '%s' AND %s < '%s'",
		quotedDateCol, quotedDateCol, quotedTable, quotedDateCol, startStr, quotedDateCol, endStr)

	var minDate, maxDate *time.Time
	err = c.conn.QueryRow(ctx, query).Scan(&minDate, &maxDate)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("failed to get min/max date: %w", err)
	}

	if minDate == nil || maxDate == nil {
		return time.Time{}, time.Time{}, nil // no rows found
	}

	return *minDate, *maxDate, nil
}
