package clickhouse

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// BatchReader reads rows from a ClickHouse query result in batches.
type BatchReader struct {
	rows      driver.Rows
	columns   []string
	colTypes  []driver.ColumnType
	batchSize int
	totalRead uint64
}

// BatchResult holds a single batch of rows.
type BatchResult struct {
	Columns []string
	Rows    [][]any
	Count   int
}

// NewBatchReader creates a reader that queries data within a specific date range and fetches in batches
func (c *Client) NewBatchReader(ctx context.Context, table string, columns []string, dateCol string, start time.Time, end time.Time, dateType DateColumnType, batchSize int) (*BatchReader, error) {
	quotedTable, err := QuoteTableName(table)
	if err != nil {
		return nil, err
	}

	var quotedColumns []string
	for _, col := range columns {
		quotedColumns = append(quotedColumns, QuoteIdentifier(col))
	}

	quotedDateCol := QuoteIdentifier(dateCol)
	startStr := FormatDateBoundary(start, dateType)
	endStr := FormatDateBoundary(end, dateType)

	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s >= '%s' AND %s < '%s'",
		strings.Join(quotedColumns, ", "), quotedTable, quotedDateCol, startStr, quotedDateCol, endStr)

	rows, err := c.conn.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to execute batch query: %w", err)
	}

	colTypes := rows.ColumnTypes()

	return &BatchReader{
		rows:      rows,
		columns:   columns,
		colTypes:  colTypes,
		batchSize: batchSize,
		totalRead: 0,
	}, nil
}

// ReadBatch reads the next batch of rows from the cursor
func (br *BatchReader) ReadBatch() (*BatchResult, error) {
	var batchRows [][]any
	count := 0

	for count < br.batchSize && br.rows.Next() {
		row := make([]any, len(br.columns))
		for i := range row {
			row[i] = reflect.New(br.colTypes[i].ScanType()).Interface()
		}

		if err := br.rows.Scan(row...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// deref the pointers
		valRow := make([]any, len(br.columns))
		for i := range row {
			valRow[i] = reflect.ValueOf(row[i]).Elem().Interface()
		}

		batchRows = append(batchRows, valRow)
		count++
	}

	if err := br.rows.Err(); err != nil {
		return nil, fmt.Errorf("error during row iteration: %w", err)
	}

	if count == 0 {
		return nil, io.EOF
	}

	br.totalRead += uint64(count)

	return &BatchResult{
		Columns: br.columns,
		Rows:    batchRows,
		Count:   count,
	}, nil
}

// TotalRead returns the total number of rows read so far
func (br *BatchReader) TotalRead() uint64 {
	return br.totalRead
}

// Close closes the underlying rows cursor
func (br *BatchReader) Close() error {
	return br.rows.Close()
}

// Columns returns the names of the columns being read
func (br *BatchReader) Columns() []string {
	return br.columns
}

// InsertBatch inserts a batch of rows into a table using the native protocol batch API
func (c *Client) InsertBatch(ctx context.Context, table string, columns []string, rows [][]any) (uint64, error) {
	if len(rows) == 0 {
		return 0, nil
	}

	quotedTable, err := QuoteTableName(table)
	if err != nil {
		return 0, err
	}

	var quotedColumns []string
	for _, col := range columns {
		quotedColumns = append(quotedColumns, QuoteIdentifier(col))
	}

	query := fmt.Sprintf("INSERT INTO %s (%s)", quotedTable, strings.Join(quotedColumns, ", "))

	batch, err := c.conn.PrepareBatch(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to prepare batch: %w", err)
	}

	for _, row := range rows {
		if err := batch.Append(row...); err != nil {
			return 0, fmt.Errorf("failed to append row to batch: %w", err)
		}
	}

	if err := batch.Send(); err != nil {
		return 0, fmt.Errorf("failed to send batch: %w", err)
	}

	return uint64(len(rows)), nil
}

// GetColumns retrieves just the names of the columns for a table, excluding ALIAS columns
func (c *Client) GetColumns(ctx context.Context, fullTableName string) ([]string, error) {
	cols, err := c.DescribeTable(ctx, fullTableName)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, col := range cols {
		if col.DefaultKind != "ALIAS" {
			names = append(names, col.Name)
		}
	}
	return names, nil
}
