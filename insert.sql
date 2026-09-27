-- ============================================================================
-- ClickHouse Test-Data Setup Script
-- Purpose: Provision 10 databases, each with one `status_90` table, covering
-- a broad range of distinct ClickHouse data types, for testing a database
-- synchronization tool (initial sync, incremental sync, date filtering,
-- delete detection, dry-run, duplicate detection, type conversion, NULL
-- handling, complex types, large batch processing, repeated/standby sync).
--
-- Each table: MergeTree engine, NO primary key, ORDER BY tuple(),
-- exactly 2500 rows, 500 rows per day across 2026-09-23 .. 2026-09-27,
-- with DateTime values spread across each day (not repeated).
-- ============================================================================

-- ----------------------------------------------------------------------------
-- Compatibility settings for newer / experimental ClickHouse types.
-- These are required on many current ClickHouse builds to enable the JSON,
-- Variant and Dynamic types used in mongodb_hub and mssql_hub below.
-- On versions where these types are already stable/default-enabled, these
-- SET statements are harmless no-ops. If your exact ClickHouse build does
-- not recognize one of these setting names, simply delete that line (and,
-- if necessary, the corresponding experimental column) before running.
-- Geo types (Point/Ring/Polygon/MultiPolygon) and AggregateFunction /
-- SimpleAggregateFunction are long-stable and need no flag.
-- ----------------------------------------------------------------------------
SET allow_experimental_json_type = 1;
SET allow_experimental_variant_type = 1;
SET allow_experimental_dynamic_type = 1;

-- ============================================================================
-- 1. DATABASE CREATION
-- ============================================================================
CREATE DATABASE IF NOT EXISTS mysql_hub;
CREATE DATABASE IF NOT EXISTS postgres_hub;
CREATE DATABASE IF NOT EXISTS mongodb_hub;
CREATE DATABASE IF NOT EXISTS oracle_hub;
CREATE DATABASE IF NOT EXISTS mssql_hub;
CREATE DATABASE IF NOT EXISTS clickhouse_hub;
CREATE DATABASE IF NOT EXISTS redis_hub;
CREATE DATABASE IF NOT EXISTS kafka_hub;
CREATE DATABASE IF NOT EXISTS mysql_replica_hub;
CREATE DATABASE IF NOT EXISTS analytics_hub;


-- ============================================================================
-- 2. TABLE CREATION
-- ============================================================================

-- ----------------------------------------------------------------------------
-- mysql_hub.status_90 : core scalar types
-- Types: Int8, Int16, Int32, Int64, UInt32, Float32, Float64, Decimal,
--        Bool, String, FixedString, Date, DateTime
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS mysql_hub.status_90
(
    id           UInt64,
    tiny_val     Int8,
    small_val    Int16,
    medium_val   Int32,
    big_val      Int64,
    unsigned_val UInt32,
    float_val    Float32,
    double_val   Float64,
    price        Decimal(10, 2),
    is_active    Bool,
    name         String,
    code         FixedString(8),
    event_date   Date,
    event_time   DateTime,
    version      UInt32,
    is_deleted   Bool
)
ENGINE = MergeTree
ORDER BY tuple();

-- ----------------------------------------------------------------------------
-- postgres_hub.status_90 : identity / network / enumerated types
-- Types: UUID, IPv4, IPv6, Enum8, Enum16, LowCardinality(String), Date, DateTime
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS postgres_hub.status_90
(
    id          UInt64,
    session_id  UUID,
    client_ip4  IPv4,
    client_ip6  IPv6,
    status      Enum8('pending' = 1, 'active' = 2, 'completed' = 3, 'failed' = 4),
    priority    Enum16('low' = 1, 'medium' = 2, 'high' = 3, 'critical' = 4),
    category    LowCardinality(String),
    region      LowCardinality(String),
    event_date  Date,
    event_time  DateTime,
    version     UInt32,
    is_deleted  Bool
)
ENGINE = MergeTree
ORDER BY tuple();

-- ----------------------------------------------------------------------------
-- mongodb_hub.status_90 : document-oriented / semi-structured types
-- Types: JSON, Array, Map, Tuple, Nested, Nullable, UUID, Date, DateTime
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS mongodb_hub.status_90
(
    id           UInt64,
    document_id  UUID,
    payload      JSON,
    tags         Array(String),
    metadata     Map(String, String),
    coordinates  Tuple(Float64, Float64),
    attributes   Nested(key String, value String),
    description  Nullable(String),
    event_date   Date,
    event_time   DateTime,
    version      UInt32,
    is_deleted   Bool
)
ENGINE = MergeTree
ORDER BY tuple();

-- ----------------------------------------------------------------------------
-- oracle_hub.status_90 : wide-precision numerics / heavy NULL handling
-- Types: Int128, UInt128, Int256, Decimal128, Decimal64, Nullable
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS oracle_hub.status_90
(
    id                  UInt64,
    huge_int            Int128,
    huge_uint           UInt128,
    massive_int         Int256,
    account_balance     Decimal(38, 10),
    transaction_amount  Decimal(18, 4),
    nullable_note       Nullable(String),
    nullable_score      Nullable(Float64),
    nullable_amount     Nullable(Decimal(10, 2)),
    event_date          Date,
    event_time          DateTime,
    version             UInt32,
    is_deleted          Bool
)
ENGINE = MergeTree
ORDER BY tuple();

-- ----------------------------------------------------------------------------
-- mssql_hub.status_90 : geo / polymorphic types
-- Types: Point, Ring, Polygon, MultiPolygon, Variant, Dynamic
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS mssql_hub.status_90
(
    id              UInt64,
    location        Point,
    boundary        Ring,
    territory       Polygon,
    regions         MultiPolygon,
    flexible_value  Variant(Int64, String, Float64),
    dynamic_value   Dynamic,
    event_date      Date,
    event_time      DateTime,
    version         UInt32,
    is_deleted      Bool
)
ENGINE = MergeTree
ORDER BY tuple();

-- ----------------------------------------------------------------------------
-- clickhouse_hub.status_90 : aggregate-state types / high-precision time
-- Types: SimpleAggregateFunction, AggregateFunction, DateTime64, LowCardinality
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS clickhouse_hub.status_90
(
    id                    UInt64,
    metric_name           LowCardinality(String),
    request_count         SimpleAggregateFunction(sum, UInt64),
    total_duration_state  AggregateFunction(sum, UInt64),
    max_latency           SimpleAggregateFunction(max, Float64),
    precise_time          DateTime64(3),
    event_date            Date,
    event_time            DateTime,
    version               UInt32,
    is_deleted            Bool
)
ENGINE = MergeTree
ORDER BY tuple();

-- ----------------------------------------------------------------------------
-- redis_hub.status_90 : key-value oriented types
-- Types: Map(String, UInt32), Nullable(DateTime), LowCardinality, Bool
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS redis_hub.status_90
(
    id               UInt64,
    cache_key        String,
    cache_value      String,
    ttl_seconds      UInt32,
    key_type         LowCardinality(String),
    attributes       Map(String, UInt32),
    nullable_expiry  Nullable(DateTime),
    is_persistent    Bool,
    event_date       Date,
    event_time       DateTime,
    version          UInt32,
    is_deleted       Bool
)
ENGINE = MergeTree
ORDER BY tuple();

-- ----------------------------------------------------------------------------
-- kafka_hub.status_90 : streaming / message types
-- Types: Array(String), Nested, UInt16, DateTime64
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS kafka_hub.status_90
(
    id             UInt64,
    topic          LowCardinality(String),
    partition_id   UInt16,
    offset_val     UInt64,
    message_keys   Array(String),
    message_values Array(String),
    headers        Nested(key String, value String),
    event_date     Date,
    event_time     DateTime64(3),
    version        UInt32,
    is_deleted     Bool
)
ENGINE = MergeTree
ORDER BY tuple();

-- ----------------------------------------------------------------------------
-- mysql_replica_hub.status_90 : mirrors mysql_hub shape + replica-lag fields
-- Adds row_checksum (duplicate detection) and updated_at (incremental sync)
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS mysql_replica_hub.status_90
(
    id            UInt64,
    tiny_val      Int8,
    medium_val    Int32,
    big_val       Int64,
    float_val     Float32,
    price         Decimal(10, 2),
    is_active     Bool,
    name          String,
    code          FixedString(8),
    row_checksum  UInt64,
    event_date    Date,
    event_time    DateTime,
    updated_at    DateTime,
    version       UInt32,
    is_deleted    Bool
)
ENGINE = MergeTree
ORDER BY tuple();

-- ----------------------------------------------------------------------------
-- analytics_hub.status_90 : wide analytical mix
-- Types: Array(Float64), Tuple(3x Float64), Decimal, Float32, FixedString(2)
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS analytics_hub.status_90
(
    id                 UInt64,
    user_id            UUID,
    metric_values      Array(Float64),
    score_breakdown    Tuple(Float64, Float64, Float64),
    conversion_rate    Decimal(5, 4),
    revenue            Decimal(12, 2),
    session_duration   Float32,
    device_type        LowCardinality(String),
    country_code       FixedString(2),
    is_converted       Bool,
    nullable_referrer  Nullable(String),
    event_date         Date,
    event_time         DateTime,
    version            UInt32,
    is_deleted         Bool
)
ENGINE = MergeTree
ORDER BY tuple();


-- ============================================================================
-- 3. DATA INSERTION  (2500 rows per table: 500 rows x 5 dates, 2026-09-23..27)
-- ============================================================================

-- ---- mysql_hub -------------------------------------------------------------
INSERT INTO mysql_hub.status_90
(
    id, tiny_val, small_val, medium_val, big_val, unsigned_val,
    float_val, double_val, price, is_active, name, code,
    event_date, event_time, version, is_deleted
)
WITH
    toDate('2026-09-27') - toIntervalDay(intDiv(number, 500)) AS event_date,
    toDateTime(event_date) + toIntervalSecond(intDiv((number % 500) * 86400, 500)) AS event_time
SELECT
    number AS id,
    toInt8((number % 200) - 100) AS tiny_val,
    toInt16((number % 40000) - 20000) AS small_val,
    toInt32(number * 37 - 1000000) AS medium_val,
    toInt64(number * 999983) AS big_val,
    toUInt32(number * 7) AS unsigned_val,
    toFloat32(number % 1000) / 3.0 AS float_val,
    toFloat64(number % 100000) / 7.0 AS double_val,
    CAST(round((number % 100000) / 100.0, 2) AS Decimal(10, 2)) AS price,
    (number % 2 = 0) AS is_active,
    concat('customer_', toString(number)) AS name,
    toFixedString(concat('CU', leftPad(toString(number % 100000), 6, '0')), 8) AS code,
    event_date,
    event_time,
    toUInt32((number % 3) + 1) AS version,
    (number % 37 = 0) AS is_deleted
FROM numbers(2500);

-- ---- postgres_hub -----------------------------------------------------------
INSERT INTO postgres_hub.status_90
(
    id, session_id, client_ip4, client_ip6, status, priority,
    category, region, event_date, event_time, version, is_deleted
)
WITH
    toDate('2026-09-27') - toIntervalDay(intDiv(number, 500)) AS event_date,
    toDateTime(event_date) + toIntervalSecond(intDiv((number % 500) * 86400, 500)) AS event_time
SELECT
    number AS id,
    generateUUIDv4() AS session_id,
    toIPv4(concat('10.', toString(intDiv(number, 256) % 256), '.', toString(intDiv(number, 16) % 256), '.', toString(number % 256))) AS client_ip4,
    toIPv6(concat('2001:db8::', hex(toUInt32(number % 65536)))) AS client_ip6,
    CAST(['pending', 'active', 'completed', 'failed'][(number % 4) + 1] AS Enum8('pending' = 1, 'active' = 2, 'completed' = 3, 'failed' = 4)) AS status,
    CAST(['low', 'medium', 'high', 'critical'][(number % 4) + 1] AS Enum16('low' = 1, 'medium' = 2, 'high' = 3, 'critical' = 4)) AS priority,
    arrayElement(['electronics', 'clothing', 'books', 'grocery', 'toys'], (number % 5) + 1) AS category,
    arrayElement(['us-east', 'us-west', 'eu-central', 'ap-south'], (number % 4) + 1) AS region,
    event_date,
    event_time,
    toUInt32((number % 3) + 1) AS version,
    (number % 37 = 0) AS is_deleted
FROM numbers(2500);

-- ---- mongodb_hub -------------------------------------------------------------
INSERT INTO mongodb_hub.status_90
(
    id, document_id, payload, tags, metadata, coordinates,
    attributes.key, attributes.value, description,
    event_date, event_time, version, is_deleted
)
WITH
    toDate('2026-09-27') - toIntervalDay(intDiv(number, 500)) AS event_date,
    toDateTime(event_date) + toIntervalSecond(intDiv((number % 500) * 86400, 500)) AS event_time
SELECT
    number AS id,
    generateUUIDv4() AS document_id,
    CAST(concat('{"id":', toString(number), ',"active":', toString(number % 2 = 0), ',"score":', toString(number % 100), '}') AS JSON) AS payload,
    [concat('tag', toString(number % 5)), concat('cat', toString(number % 3)), concat('flag', toString(number % 7))] AS tags,
    map('region', concat('r', toString(number % 5)), 'zone', concat('z', toString(number % 3))) AS metadata,
    (toFloat64(number % 180) - 90.0, toFloat64(number % 360) - 180.0) AS coordinates,
       ['k1', 'k2'] AS attr_keys,
    [concat('v1_', toString(number)), concat('v2_', toString(number))] AS attr_values,
            if(number % 7 = 0, NULL, concat('doc_note_', toString(number))) AS description,
    event_date,
    event_time,
    toUInt32((number % 3) + 1) AS version,
    (number % 37 = 0) AS is_deleted
FROM numbers(2500);

-- ---- oracle_hub -------------------------------------------------------------
INSERT INTO oracle_hub.status_90
(
    id, huge_int, huge_uint, massive_int, account_balance, transaction_amount,
    nullable_note, nullable_score, nullable_amount,
    event_date, event_time, version, is_deleted
)
WITH
    toDate('2026-09-27') - toIntervalDay(intDiv(number, 500)) AS event_date,
    toDateTime(event_date) + toIntervalSecond(intDiv((number % 500) * 86400, 500)) AS event_time
SELECT
    number AS id,
    toInt128(number) * 100000000000 AS huge_int,
    toUInt128(number) * 200000000000 AS huge_uint,
    toInt256(number) * 100000000000000000 AS massive_int,
    CAST(round((number * 1234.56789) / 3, 10) AS Decimal(38, 10)) AS account_balance,
    CAST(round((number % 100000) / 3.0, 4) AS Decimal(18, 4)) AS transaction_amount,
    if(number % 5 = 0, NULL, concat('note_', toString(number))) AS nullable_note,
    if(number % 6 = 0, NULL, toFloat64(number % 1000) / 9.0) AS nullable_score,
    if(number % 8 = 0, NULL, CAST(round((number % 5000) / 100.0, 2) AS Decimal(10, 2))) AS nullable_amount,
    event_date,
    event_time,
    toUInt32((number % 3) + 1) AS version,
    (number % 37 = 0) AS is_deleted
FROM numbers(2500);

-- ---- mssql_hub -------------------------------------------------------------
INSERT INTO mssql_hub.status_90
(
    id, location, boundary, territory, regions, flexible_value, dynamic_value,
    event_date, event_time, version, is_deleted
)
WITH
    toDate('2026-09-27') - toIntervalDay(intDiv(number, 500)) AS event_date,
    toDateTime(event_date) + toIntervalSecond(intDiv((number % 500) * 86400, 500)) AS event_time
SELECT
    number AS id,
    (toFloat64(number % 180) - 90.0, toFloat64(number % 360) - 180.0) AS location,
    [(0.0, 0.0), (0.0, 1.0 + (number % 10) / 10.0), (1.0, 1.0), (1.0, 0.0)] AS boundary,
    [[(0.0, 0.0), (0.0, 2.0), (2.0, 2.0), (2.0, 0.0)]] AS territory,
    [[[(0.0, 0.0), (0.0, 3.0), (3.0, 3.0)]], [[(5.0, 5.0), (5.0, 6.0), (6.0, 6.0)]]] AS regions,
    multiIf(
        number % 3 = 0, CAST(toInt64(number) AS Variant(Int64, String, Float64)),
        number % 3 = 1, CAST(concat('var_', toString(number)) AS Variant(Int64, String, Float64)),
        CAST(toFloat64(number) / 3.0 AS Variant(Int64, String, Float64))
    ) AS flexible_value,
    multiIf(
        number % 4 = 0, CAST(toInt64(number) AS Dynamic),
        number % 4 = 1, CAST(concat('dyn_', toString(number)) AS Dynamic),
        number % 4 = 2, CAST(toFloat64(number) / 7.0 AS Dynamic),
        CAST(generateUUIDv4() AS Dynamic)
    ) AS dynamic_value,
    event_date,
    event_time,
    toUInt32((number % 3) + 1) AS version,
    (number % 37 = 0) AS is_deleted
FROM numbers(2500);

-- ---- clickhouse_hub -------------------------------------------------------------
INSERT INTO clickhouse_hub.status_90
(
    id, metric_name, request_count, total_duration_state, max_latency, precise_time,
    event_date, event_time, version, is_deleted
)
WITH
    toDate('2026-09-27') - toIntervalDay(intDiv(number, 500)) AS event_date,
    toDateTime(event_date) + toIntervalSecond(intDiv((number % 500) * 86400, 500)) AS event_time
SELECT
    number AS id,
    arrayElement(['cpu_usage', 'mem_usage', 'disk_io', 'net_io'], (number % 4) + 1) AS metric_name,
    toUInt64(number % 50 + 1) AS request_count,
    initializeAggregation('sumState', toUInt64(number % 1000 + 1)) AS total_duration_state,
    toFloat64(number % 500) * 0.37 AS max_latency,
    toDateTime64(event_time, 3) + toIntervalMillisecond(number % 1000) AS precise_time,
    event_date,
    event_time,
    toUInt32((number % 3) + 1) AS version,
    (number % 37 = 0) AS is_deleted
FROM numbers(2500);

-- ---- redis_hub -------------------------------------------------------------
INSERT INTO redis_hub.status_90
(
    id, cache_key, cache_value, ttl_seconds, key_type, attributes,
    nullable_expiry, is_persistent, event_date, event_time, version, is_deleted
)
WITH
    toDate('2026-09-27') - toIntervalDay(intDiv(number, 500)) AS event_date,
    toDateTime(event_date) + toIntervalSecond(intDiv((number % 500) * 86400, 500)) AS event_time
SELECT
    number AS id,
    concat('key:', toString(number)) AS cache_key,
    concat('value_payload_', toString(number * 3)) AS cache_value,
    toUInt32((number % 3600) + 60) AS ttl_seconds,
    arrayElement(['string', 'hash', 'list', 'set', 'zset'], (number % 5) + 1) AS key_type,
    map('hits', toUInt32(number % 100), 'misses', toUInt32(number % 10)) AS attributes,
    if(number % 4 = 0, NULL, event_time + toIntervalHour(24)) AS nullable_expiry,
    (number % 2 = 1) AS is_persistent,
    event_date,
    event_time,
    toUInt32((number % 3) + 1) AS version,
    (number % 37 = 0) AS is_deleted
FROM numbers(2500);

-- ---- kafka_hub -------------------------------------------------------------
INSERT INTO kafka_hub.status_90
(
    id, topic, partition_id, offset_val, message_keys, message_values,
    headers.key, headers.value, event_date, event_time, version, is_deleted
)
WITH
    toDate('2026-09-27') - toIntervalDay(intDiv(number, 500)) AS event_date,
    toDateTime(event_date) + toIntervalSecond(intDiv((number % 500) * 86400, 500)) AS base_event_time
SELECT
    number AS id,
    arrayElement(['orders', 'payments', 'inventory', 'notifications'], (number % 4) + 1) AS topic,
    toUInt16(number % 12) AS partition_id,
    toUInt64(number * 17) AS offset_val,
    [concat('k', toString(number % 5)), concat('k', toString((number + 1) % 5))] AS message_keys,
    [concat('msg_', toString(number)), concat('retry_', toString(number % 3))] AS message_values,
    ['source', 'trace-id'] AS hdr_keys,
    [concat('svc-', toString(number % 8)), concat('trc-', toString(number))] AS hdr_values,
    event_date,
    toDateTime64(base_event_time, 3) + toIntervalMillisecond(number % 1000) AS event_time,
    toUInt32((number % 3) + 1) AS version,
    (number % 37 = 0) AS is_deleted
FROM numbers(2500);

-- ---- mysql_replica_hub -------------------------------------------------------------
INSERT INTO mysql_replica_hub.status_90
(
    id, tiny_val, medium_val, big_val, float_val, price, is_active, name, code,
    row_checksum, event_date, event_time, updated_at, version, is_deleted
)
WITH
    toDate('2026-09-27') - toIntervalDay(intDiv(number, 500)) AS event_date,
    toDateTime(event_date) + toIntervalSecond(intDiv((number % 500) * 86400, 500)) AS event_time
SELECT
    number AS id,
    toInt8((number % 200) - 100) AS tiny_val,
    toInt32(number * 41 - 500000) AS medium_val,
    toInt64(number * 888887) AS big_val,
    toFloat32(number % 1000) / 4.0 AS float_val,
    CAST(round((number % 100000) / 100.0, 2) AS Decimal(10, 2)) AS price,
    (number % 2 = 0) AS is_active,
    concat('customer_', toString(number)) AS name,
    toFixedString(concat('CU', leftPad(toString(number % 100000), 6, '0')), 8) AS code,
    cityHash64(concat(toString(number), toString(number * 41 - 500000))) AS row_checksum,
    event_date,
    event_time,
    event_time + toIntervalMinute(number % 60) AS updated_at,
    toUInt32((number % 3) + 1) AS version,
    (number % 37 = 0) AS is_deleted
FROM numbers(2500);

-- ---- analytics_hub -------------------------------------------------------------
INSERT INTO analytics_hub.status_90
(
    id, user_id, metric_values, score_breakdown, conversion_rate, revenue,
    session_duration, device_type, country_code, is_converted, nullable_referrer,
    event_date, event_time, version, is_deleted
)
WITH
    toDate('2026-09-27') - toIntervalDay(intDiv(number, 500)) AS event_date,
    toDateTime(event_date) + toIntervalSecond(intDiv((number % 500) * 86400, 500)) AS event_time
SELECT
    number AS id,
    generateUUIDv4() AS user_id,
    [toFloat64(number % 100) / 10.0, toFloat64(number % 50) / 5.0, toFloat64(number % 30) / 3.0] AS metric_values,
    (toFloat64(number % 100) / 10.0, toFloat64(number % 80) / 10.0, toFloat64(number % 60) / 10.0) AS score_breakdown,
    CAST(round((number % 10000) / 10000.0, 4) AS Decimal(5, 4)) AS conversion_rate,
    CAST(round((number % 1000000) / 100.0, 2) AS Decimal(12, 2)) AS revenue,
    toFloat32(number % 3600) / 10.0 AS session_duration,
    arrayElement(['mobile', 'desktop', 'tablet', 'tv'], (number % 4) + 1) AS device_type,
    toFixedString(arrayElement(['US', 'GB', 'DE', 'IN', 'JP'], (number % 5) + 1), 2) AS country_code,
    (number % 3 = 0) AS is_converted,
    if(number % 9 = 0, NULL, arrayElement(['google', 'facebook', 'direct', 'newsletter'], (number % 4) + 1)) AS nullable_referrer,
    event_date,
    event_time,
    toUInt32((number % 3) + 1) AS version,
    (number % 37 = 0) AS is_deleted
FROM numbers(2500);


-- ============================================================================
-- 4. VALIDATION QUERIES
-- ============================================================================

-- ---- 4a. Total row count per table (expect 2500 for every row) ------------
SELECT 'mysql_hub' AS db, count() AS total_rows FROM mysql_hub.status_90
UNION ALL
SELECT 'postgres_hub', count() FROM postgres_hub.status_90
UNION ALL
SELECT 'mongodb_hub', count() FROM mongodb_hub.status_90
UNION ALL
SELECT 'oracle_hub', count() FROM oracle_hub.status_90
UNION ALL
SELECT 'mssql_hub', count() FROM mssql_hub.status_90
UNION ALL
SELECT 'clickhouse_hub', count() FROM clickhouse_hub.status_90
UNION ALL
SELECT 'redis_hub', count() FROM redis_hub.status_90
UNION ALL
SELECT 'kafka_hub', count() FROM kafka_hub.status_90
UNION ALL
SELECT 'mysql_replica_hub', count() FROM mysql_replica_hub.status_90
UNION ALL
SELECT 'analytics_hub', count() FROM analytics_hub.status_90
ORDER BY db;

-- ---- 4b. Rows per date per table (expect 500 for each of the 5 dates) -----
SELECT 'mysql_hub' AS db, event_date, count() AS row_count FROM mysql_hub.status_90 GROUP BY event_date
UNION ALL
SELECT 'postgres_hub', event_date, count() FROM postgres_hub.status_90 GROUP BY event_date
UNION ALL
SELECT 'mongodb_hub', event_date, count() FROM mongodb_hub.status_90 GROUP BY event_date
UNION ALL
SELECT 'oracle_hub', event_date, count() FROM oracle_hub.status_90 GROUP BY event_date
UNION ALL
SELECT 'mssql_hub', event_date, count() FROM mssql_hub.status_90 GROUP BY event_date
UNION ALL
SELECT 'clickhouse_hub', event_date, count() FROM clickhouse_hub.status_90 GROUP BY event_date
UNION ALL
SELECT 'redis_hub', event_date, count() FROM redis_hub.status_90 GROUP BY event_date
UNION ALL
SELECT 'kafka_hub', event_date, count() FROM kafka_hub.status_90 GROUP BY event_date
UNION ALL
SELECT 'mysql_replica_hub', event_date, count() FROM mysql_replica_hub.status_90 GROUP BY event_date
UNION ALL
SELECT 'analytics_hub', event_date, count() FROM analytics_hub.status_90 GROUP BY event_date
ORDER BY db, event_date;

-- ---- 4c. DateTime range per table (expect min ~2026-09-23 00:00:00,
--          max ~2026-09-27 23:59:xx) ----------------------------------------
SELECT 'mysql_hub' AS db, min(event_time) AS min_dt, max(event_time) AS max_dt FROM mysql_hub.status_90
UNION ALL
SELECT 'postgres_hub', min(event_time), max(event_time) FROM postgres_hub.status_90
UNION ALL
SELECT 'mongodb_hub', min(event_time), max(event_time) FROM mongodb_hub.status_90
UNION ALL
SELECT 'oracle_hub', min(event_time), max(event_time) FROM oracle_hub.status_90
UNION ALL
SELECT 'mssql_hub', min(event_time), max(event_time) FROM mssql_hub.status_90
UNION ALL
SELECT 'clickhouse_hub', min(event_time), max(event_time) FROM clickhouse_hub.status_90
UNION ALL
SELECT 'redis_hub', min(event_time), max(event_time) FROM redis_hub.status_90
UNION ALL
SELECT 'kafka_hub', min(toDateTime(event_time)), max(toDateTime(event_time)) FROM kafka_hub.status_90
UNION ALL
SELECT 'mysql_replica_hub', min(event_time), max(event_time) FROM mysql_replica_hub.status_90
UNION ALL
SELECT 'analytics_hub', min(event_time), max(event_time) FROM analytics_hub.status_90
ORDER BY db;

-- ---- 4d. Duplicate / uniqueness check on id (expect 2500 distinct ids) ----
SELECT 'mysql_hub' AS db, uniqExact(id) AS distinct_ids FROM mysql_hub.status_90
UNION ALL
SELECT 'postgres_hub', uniqExact(id) FROM postgres_hub.status_90
UNION ALL
SELECT 'mongodb_hub', uniqExact(id) FROM mongodb_hub.status_90
UNION ALL
SELECT 'oracle_hub', uniqExact(id) FROM oracle_hub.status_90
UNION ALL
SELECT 'mssql_hub', uniqExact(id) FROM mssql_hub.status_90
UNION ALL
SELECT 'clickhouse_hub', uniqExact(id) FROM clickhouse_hub.status_90
UNION ALL
SELECT 'redis_hub', uniqExact(id) FROM redis_hub.status_90
UNION ALL
SELECT 'kafka_hub', uniqExact(id) FROM kafka_hub.status_90
UNION ALL
SELECT 'mysql_replica_hub', uniqExact(id) FROM mysql_replica_hub.status_90
UNION ALL
SELECT 'analytics_hub', uniqExact(id) FROM analytics_hub.status_90
ORDER BY db;

-- ---- 4e. Schema / type inspection via system metadata ----------------------
SELECT database, table, name AS column_name, type AS column_type, position
FROM system.columns
WHERE database IN (
        'mysql_hub', 'postgres_hub', 'mongodb_hub', 'oracle_hub', 'mssql_hub',
        'clickhouse_hub', 'redis_hub', 'kafka_hub', 'mysql_replica_hub', 'analytics_hub'
      )
  AND table = 'status_90'
ORDER BY database, position;

-- ---- 4f. Sample aggregate-state readback (clickhouse_hub only) ------------
SELECT
    id,
    metric_name,
    request_count,
    sumMerge(total_duration_state) OVER (PARTITION BY id) AS total_duration_readback,
    max_latency,
    precise_time
FROM clickhouse_hub.status_90
LIMIT 5;
