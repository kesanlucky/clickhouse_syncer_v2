# ClickHouse Data Syncer

## Overview
A production-quality Go CLI application that synchronizes predefined tables between two ClickHouse server instances. The Go application reads data from a source ClickHouse server and writes it to a destination ClickHouse server. It does NOT use ClickHouse server-side remote() or INSERT SELECT mechanisms.

Architecture diagram:
```
SOURCE ClickHouse → Go Syncer Application → DESTINATION ClickHouse
```

## Features
- Date-specific data synchronization
- Idempotent sync operations
- Bounded concurrency
- Schema validation before any data modification
- Safe deletion with confirmation
- Dry-run mode
- Standby (continuous) sync mode
- Configurable retry with exponential backoff
- Structured logging with rotation
- Graceful shutdown (SIGINT/SIGTERM)
- Comprehensive verification after operations

## Requirements
- Go 1.21+
- Two ClickHouse server instances
- Tables with identical schemas on both servers

## Installation
```bash
git clone <repo>
cd clickhouse-syncer
go build -o clickhouse-syncer ./cmd/syncer/
```

## Configuration
Full `config.yaml` reference with all fields documented:
- `source`/`destination`: `host`, `port`, `database`, `user`, `password`
- `timezone` (default: Asia/Kolkata)
- `sync`: `batch_size`, `max_concurrency`, `standby` (`enabled`, `interval`), `retries` (`enabled`, `max_attempts`, `backoff`)
- `logging`: `level`, `directory`, `console`
- `tables`: `name`, `date_column`

### Example `config.yaml`
```yaml
source:
  host: "127.0.0.1"
  port: 9000
  database: "default"
  user: "default"
  password: ""

destination:
  host: "127.0.0.1"
  port: 9001
  database: "default"
  user: "default"
  password: ""

timezone: "Asia/Kolkata"

sync:
  batch_size: 50000
  max_concurrency: 4
  standby:
    enabled: true
    interval: "1m"
  retries:
    enabled: true
    max_attempts: 3
    backoff: "1s"

logging:
  level: "info"
  directory: "./logs"
  console: true

tables:
  - name: "system.query_log"
    date_column: "event_date"
```

## CLI Usage

### Validate
Verifies connections, schemas, and configurations without modifying any data.
```bash
./clickhouse-syncer validate
```

### Sync
Synchronizes data for a specific date.
```bash
./clickhouse-syncer sync --date 2026-09-25
./clickhouse-syncer sync --date 2026-09-25 --table postgresql_hub.status_90
./clickhouse-syncer sync --date 2026-09-25 --all
```

### Delete
Deletes data from the destination server before a specific date.
```bash
./clickhouse-syncer delete --before 2026-09-25
./clickhouse-syncer delete --before 2026-09-25 --table postgresql_hub.status_90
./clickhouse-syncer delete --before 2026-09-25 --force
```

### Dry Run
Simulates operations without performing any data modification.
```bash
./clickhouse-syncer dry-run sync --date 2026-09-25
./clickhouse-syncer dry-run delete --before 2026-09-25
```

### Standby Sync
Runs continuous synchronization in the background.
```bash
./clickhouse-syncer standby-sync
```

## How Sync Works
1. **Validation**: All connection and schema validations pass first.
2. **Table Processing**: For each table:
   - Count source rows for the specific date.
   - Delete any existing destination data for that date.
   - Wait for the destination mutation to complete.
   - Transfer data in batches.
   - Verify counts on the destination match the source.
3. **Date Handling**: Uses half-open date intervals (`>= start AND < end`) to ensure precision without timezone/boundary overlaps.
4. **No BETWEEN**: Never uses `BETWEEN` for datetime ranges.
5. **Idempotent**: It is safe to run repeatedly. Syncing the same date twice simply replaces the data.

## How Delete Works
1. Only operates on the **DESTINATION** server (source data is never deleted).
2. Requires `--force` flag or interactive confirmation via prompt.
3. Uses `ALTER TABLE ... DELETE WHERE` for fast, asynchronous deletion.
4. Waits for the mutation completion.
5. Verifies deletion success.

## Concurrency
- `max_concurrency` controls the number of parallel table syncs.
- Employs a bounded worker pool using a semaphore pattern.
- Ensures no unlimited goroutine spawning to protect memory and connections.

## Validation
The application performs strict validation before any data modification:
- Source connection health check
- Destination connection health check
- Schema parity (columns, types) between source and destination tables
- Configuration validation (missing fields, invalid dates)

## Logging
- Structured JSON logging to file, human-readable logging to console.
- Dedicated error log file (`error.log`).
- Log rotation (100MB max size, 30 days retention, 10 backups).
- Operation IDs injected for distributed tracing and tracking.
- Progress reporting for large tables.

## Exit Codes
| Code | Meaning |
|------|-------------------------|
| 0 | Success |
| 1 | General/runtime failure |
| 2 | Configuration error |
| 3 | Validation failure |
| 4 | Connection failure |
| 5 | Schema mismatch |
| 6 | Sync failure |
| 7 | Verification failure |
| 8 | Delete failure |

## Example Output

**Sync Output:**
```
INFO [2026-09-25 10:00:00] Starting sync operation [id: abc-123] date=2026-09-25
INFO [2026-09-25 10:00:01] Validation successful
INFO [2026-09-25 10:00:02] Syncing table status_90
INFO [2026-09-25 10:00:05] Table status_90 synced successfully (50,000 rows)
INFO [2026-09-25 10:00:06] Sync operation [id: abc-123] completed successfully
```

**Delete Output:**
```
WARNING [2026-09-25 10:05:00] Starting delete operation [id: def-456] before=2026-09-25
Are you sure you want to delete data before 2026-09-25 on the destination server? (y/N): y
INFO [2026-09-25 10:05:05] Deleting data from table status_90
INFO [2026-09-25 10:05:10] Table status_90 data deleted successfully
INFO [2026-09-25 10:05:11] Delete operation [id: def-456] completed successfully
```

**Standby Output:**
```
INFO [2026-09-25 10:10:00] Starting standby sync mode
INFO [2026-09-25 10:10:01] Running sync cycle for today (2026-09-25)
...
INFO [2026-09-25 10:11:00] Next sync cycle in 1m0s
```

## Safety Considerations
- All identifiers (table names, columns) are safely quoted.
- No raw SQL concatenation (uses parameterized queries).
- Source data is never deleted or modified.
- Schema differences immediately abort the operation.
- Delete requires explicit confirmation.
- Timezone is explicitly configured; never relies on machine local time.

## Testing
Run unit tests:
```bash
go test ./...
```
Integration testing requires a ClickHouse container setup (e.g., via `docker-compose`):
```bash
docker-compose up -d
go test -tags integration ./...
```

## Troubleshooting
- **Connection Refused**: Ensure ClickHouse is running and ports in `config.yaml` are correct (typically 9000 for native TCP).
- **Schema Mismatch**: Verify that tables on both servers are created with identical schemas. The syncer aborts on column differences.
- **Out of Memory (OOM)**: Reduce `batch_size` or `max_concurrency` in the configuration.
- **Timezone Issues**: Explicitly set the correct `timezone` in `config.yaml` to ensure date boundaries are evaluated correctly.

## Project Structure
```
.
├── cmd/
│   └── syncer/
│       └── main.go       # Application entry point
├── internal/
│   ├── app/              # CLI commands and lifecycle
│   ├── config/           # Configuration structures and parsing
│   ├── db/               # ClickHouse client and operations
│   ├── domain/           # Core logic (sync, delete, validation)
│   └── logger/           # Structured logging
├── config.yaml           # Default configuration file
├── docker-compose.yml    # For integration testing
├── go.mod
├── go.sum
└── README.md
```

## License
MIT
