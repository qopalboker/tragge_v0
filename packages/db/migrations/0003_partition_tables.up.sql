-- 0003_partition_tables.up.sql
-- Database partitioning for high-volume tables: orders, fills, positions
-- Uses LIST partitioning by shard_id for optimal query routing

-- ============================================================================
-- HELPER FUNCTION: COMPUTE SHARD ID FROM CONTEST
-- ============================================================================

-- Function to compute shard_id from contest_id (looks up from contests table)
CREATE OR REPLACE FUNCTION get_shard_id_for_contest(p_contest_id UUID)
RETURNS INT AS $$
DECLARE
    v_shard_id INT;
BEGIN
    SELECT COALESCE(shard_id, 0) INTO v_shard_id
    FROM contests
    WHERE id = p_contest_id;

    IF v_shard_id IS NULL THEN
        RETURN 0; -- Default shard for unknown contests
    END IF;

    RETURN v_shard_id;
END;
$$ LANGUAGE plpgsql STABLE;

-- ============================================================================
-- PARTITIONED ORDERS TABLE
-- ============================================================================

-- Create new partitioned orders table
CREATE TABLE orders_partitioned (
    order_id UUID NOT NULL DEFAULT uuid_generate_v4(),
    shard_id INT NOT NULL DEFAULT 0,
    contest_id UUID NOT NULL,
    user_id UUID NOT NULL,
    symbol VARCHAR(20) NOT NULL,
    side order_side NOT NULL,
    type order_type NOT NULL,
    qty BIGINT NOT NULL,
    qty_filled BIGINT NOT NULL DEFAULT 0,
    limit_price NUMERIC(20, 8),
    stop_price NUMERIC(20, 8),
    take_profit NUMERIC(20, 8),
    stop_loss NUMERIC(20, 8),
    status order_status NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Primary key must include partition key
    PRIMARY KEY (shard_id, order_id),

    -- Constraints
    CONSTRAINT chk_orders_p_qty_positive CHECK (qty > 0),
    CONSTRAINT chk_orders_p_qty_filled CHECK (qty_filled >= 0 AND qty_filled <= qty),
    CONSTRAINT chk_orders_p_limit_price_for_limit CHECK (
        (type NOT IN ('limit', 'stop_limit')) OR (limit_price IS NOT NULL AND limit_price > 0)
    ),
    CONSTRAINT chk_orders_p_stop_price_for_stop CHECK (
        (type NOT IN ('stop', 'stop_limit')) OR (stop_price IS NOT NULL AND stop_price > 0)
    ),

    -- Foreign key to shard_config
    CONSTRAINT fk_orders_p_shard FOREIGN KEY (shard_id) REFERENCES shard_config(shard_id)
) PARTITION BY LIST (shard_id);

-- Create partitions for 4 shards
CREATE TABLE orders_shard_0 PARTITION OF orders_partitioned FOR VALUES IN (0);
CREATE TABLE orders_shard_1 PARTITION OF orders_partitioned FOR VALUES IN (1);
CREATE TABLE orders_shard_2 PARTITION OF orders_partitioned FOR VALUES IN (2);
CREATE TABLE orders_shard_3 PARTITION OF orders_partitioned FOR VALUES IN (3);

-- Create indexes on each partition for optimal query performance
-- Shard 0 indexes
CREATE INDEX idx_orders_s0_contest ON orders_shard_0(contest_id);
CREATE INDEX idx_orders_s0_user ON orders_shard_0(user_id);
CREATE INDEX idx_orders_s0_symbol ON orders_shard_0(symbol);
CREATE INDEX idx_orders_s0_status ON orders_shard_0(status);
CREATE INDEX idx_orders_s0_created_at ON orders_shard_0(created_at);
CREATE INDEX idx_orders_s0_contest_user ON orders_shard_0(contest_id, user_id);
CREATE INDEX idx_orders_s0_contest_user_status ON orders_shard_0(contest_id, user_id, status);
CREATE INDEX idx_orders_s0_contest_symbol ON orders_shard_0(contest_id, symbol);
CREATE UNIQUE INDEX idx_orders_s0_order_id ON orders_shard_0(order_id);

-- Shard 1 indexes
CREATE INDEX idx_orders_s1_contest ON orders_shard_1(contest_id);
CREATE INDEX idx_orders_s1_user ON orders_shard_1(user_id);
CREATE INDEX idx_orders_s1_symbol ON orders_shard_1(symbol);
CREATE INDEX idx_orders_s1_status ON orders_shard_1(status);
CREATE INDEX idx_orders_s1_created_at ON orders_shard_1(created_at);
CREATE INDEX idx_orders_s1_contest_user ON orders_shard_1(contest_id, user_id);
CREATE INDEX idx_orders_s1_contest_user_status ON orders_shard_1(contest_id, user_id, status);
CREATE INDEX idx_orders_s1_contest_symbol ON orders_shard_1(contest_id, symbol);
CREATE UNIQUE INDEX idx_orders_s1_order_id ON orders_shard_1(order_id);

-- Shard 2 indexes
CREATE INDEX idx_orders_s2_contest ON orders_shard_2(contest_id);
CREATE INDEX idx_orders_s2_user ON orders_shard_2(user_id);
CREATE INDEX idx_orders_s2_symbol ON orders_shard_2(symbol);
CREATE INDEX idx_orders_s2_status ON orders_shard_2(status);
CREATE INDEX idx_orders_s2_created_at ON orders_shard_2(created_at);
CREATE INDEX idx_orders_s2_contest_user ON orders_shard_2(contest_id, user_id);
CREATE INDEX idx_orders_s2_contest_user_status ON orders_shard_2(contest_id, user_id, status);
CREATE INDEX idx_orders_s2_contest_symbol ON orders_shard_2(contest_id, symbol);
CREATE UNIQUE INDEX idx_orders_s2_order_id ON orders_shard_2(order_id);

-- Shard 3 indexes
CREATE INDEX idx_orders_s3_contest ON orders_shard_3(contest_id);
CREATE INDEX idx_orders_s3_user ON orders_shard_3(user_id);
CREATE INDEX idx_orders_s3_symbol ON orders_shard_3(symbol);
CREATE INDEX idx_orders_s3_status ON orders_shard_3(status);
CREATE INDEX idx_orders_s3_created_at ON orders_shard_3(created_at);
CREATE INDEX idx_orders_s3_contest_user ON orders_shard_3(contest_id, user_id);
CREATE INDEX idx_orders_s3_contest_user_status ON orders_shard_3(contest_id, user_id, status);
CREATE INDEX idx_orders_s3_contest_symbol ON orders_shard_3(contest_id, symbol);
CREATE UNIQUE INDEX idx_orders_s3_order_id ON orders_shard_3(order_id);

-- ============================================================================
-- PARTITIONED FILLS TABLE
-- ============================================================================

-- Create new partitioned fills table
CREATE TABLE fills_partitioned (
    fill_id UUID NOT NULL DEFAULT uuid_generate_v4(),
    shard_id INT NOT NULL DEFAULT 0,
    order_id UUID NOT NULL,
    contest_id UUID NOT NULL,
    user_id UUID NOT NULL,
    symbol VARCHAR(20) NOT NULL,
    side order_side NOT NULL,
    qty BIGINT NOT NULL,
    fill_price NUMERIC(20, 8) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Primary key must include partition key
    PRIMARY KEY (shard_id, fill_id),

    -- Constraints
    CONSTRAINT chk_fills_p_qty_positive CHECK (qty > 0),
    CONSTRAINT chk_fills_p_price_positive CHECK (fill_price > 0),

    -- Foreign key to shard_config
    CONSTRAINT fk_fills_p_shard FOREIGN KEY (shard_id) REFERENCES shard_config(shard_id)
) PARTITION BY LIST (shard_id);

-- Create partitions for 4 shards
CREATE TABLE fills_shard_0 PARTITION OF fills_partitioned FOR VALUES IN (0);
CREATE TABLE fills_shard_1 PARTITION OF fills_partitioned FOR VALUES IN (1);
CREATE TABLE fills_shard_2 PARTITION OF fills_partitioned FOR VALUES IN (2);
CREATE TABLE fills_shard_3 PARTITION OF fills_partitioned FOR VALUES IN (3);

-- Create indexes on each partition
-- Shard 0 indexes
CREATE INDEX idx_fills_s0_order ON fills_shard_0(order_id);
CREATE INDEX idx_fills_s0_contest ON fills_shard_0(contest_id);
CREATE INDEX idx_fills_s0_user ON fills_shard_0(user_id);
CREATE INDEX idx_fills_s0_symbol ON fills_shard_0(symbol);
CREATE INDEX idx_fills_s0_created_at ON fills_shard_0(created_at);
CREATE INDEX idx_fills_s0_contest_user ON fills_shard_0(contest_id, user_id);
CREATE INDEX idx_fills_s0_contest_symbol ON fills_shard_0(contest_id, symbol);
CREATE UNIQUE INDEX idx_fills_s0_fill_id ON fills_shard_0(fill_id);

-- Shard 1 indexes
CREATE INDEX idx_fills_s1_order ON fills_shard_1(order_id);
CREATE INDEX idx_fills_s1_contest ON fills_shard_1(contest_id);
CREATE INDEX idx_fills_s1_user ON fills_shard_1(user_id);
CREATE INDEX idx_fills_s1_symbol ON fills_shard_1(symbol);
CREATE INDEX idx_fills_s1_created_at ON fills_shard_1(created_at);
CREATE INDEX idx_fills_s1_contest_user ON fills_shard_1(contest_id, user_id);
CREATE INDEX idx_fills_s1_contest_symbol ON fills_shard_1(contest_id, symbol);
CREATE UNIQUE INDEX idx_fills_s1_fill_id ON fills_shard_1(fill_id);

-- Shard 2 indexes
CREATE INDEX idx_fills_s2_order ON fills_shard_2(order_id);
CREATE INDEX idx_fills_s2_contest ON fills_shard_2(contest_id);
CREATE INDEX idx_fills_s2_user ON fills_shard_2(user_id);
CREATE INDEX idx_fills_s2_symbol ON fills_shard_2(symbol);
CREATE INDEX idx_fills_s2_created_at ON fills_shard_2(created_at);
CREATE INDEX idx_fills_s2_contest_user ON fills_shard_2(contest_id, user_id);
CREATE INDEX idx_fills_s2_contest_symbol ON fills_shard_2(contest_id, symbol);
CREATE UNIQUE INDEX idx_fills_s2_fill_id ON fills_shard_2(fill_id);

-- Shard 3 indexes
CREATE INDEX idx_fills_s3_order ON fills_shard_3(order_id);
CREATE INDEX idx_fills_s3_contest ON fills_shard_3(contest_id);
CREATE INDEX idx_fills_s3_user ON fills_shard_3(user_id);
CREATE INDEX idx_fills_s3_symbol ON fills_shard_3(symbol);
CREATE INDEX idx_fills_s3_created_at ON fills_shard_3(created_at);
CREATE INDEX idx_fills_s3_contest_user ON fills_shard_3(contest_id, user_id);
CREATE INDEX idx_fills_s3_contest_symbol ON fills_shard_3(contest_id, symbol);
CREATE UNIQUE INDEX idx_fills_s3_fill_id ON fills_shard_3(fill_id);

-- ============================================================================
-- PARTITIONED POSITIONS TABLE
-- ============================================================================

-- Create new partitioned positions table
CREATE TABLE positions_partitioned (
    position_id UUID NOT NULL DEFAULT uuid_generate_v4(),
    shard_id INT NOT NULL DEFAULT 0,
    contest_id UUID NOT NULL,
    user_id UUID NOT NULL,
    symbol VARCHAR(20) NOT NULL,
    side position_side NOT NULL,
    qty_open BIGINT NOT NULL DEFAULT 0,
    entry_price NUMERIC(20, 8) NOT NULL,
    qty_used BIGINT NOT NULL DEFAULT 0,
    realized_score NUMERIC(20, 4) NOT NULL DEFAULT 0,
    opened_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at TIMESTAMPTZ,

    -- Primary key must include partition key
    PRIMARY KEY (shard_id, position_id),

    -- Constraints
    CONSTRAINT chk_positions_p_qty_open CHECK (qty_open >= 0),
    CONSTRAINT chk_positions_p_entry_price CHECK (entry_price > 0),
    CONSTRAINT chk_positions_p_qty_used CHECK (qty_used >= 0),

    -- Foreign key to shard_config
    CONSTRAINT fk_positions_p_shard FOREIGN KEY (shard_id) REFERENCES shard_config(shard_id)
) PARTITION BY LIST (shard_id);

-- Create partitions for 4 shards
CREATE TABLE positions_shard_0 PARTITION OF positions_partitioned FOR VALUES IN (0);
CREATE TABLE positions_shard_1 PARTITION OF positions_partitioned FOR VALUES IN (1);
CREATE TABLE positions_shard_2 PARTITION OF positions_partitioned FOR VALUES IN (2);
CREATE TABLE positions_shard_3 PARTITION OF positions_partitioned FOR VALUES IN (3);

-- Create indexes on each partition
-- Shard 0 indexes
CREATE INDEX idx_positions_s0_contest ON positions_shard_0(contest_id);
CREATE INDEX idx_positions_s0_user ON positions_shard_0(user_id);
CREATE INDEX idx_positions_s0_symbol ON positions_shard_0(symbol);
CREATE INDEX idx_positions_s0_opened_at ON positions_shard_0(opened_at);
CREATE INDEX idx_positions_s0_closed_at ON positions_shard_0(closed_at);
CREATE INDEX idx_positions_s0_contest_user ON positions_shard_0(contest_id, user_id);
CREATE INDEX idx_positions_s0_contest_user_symbol ON positions_shard_0(contest_id, user_id, symbol);
CREATE INDEX idx_positions_s0_open ON positions_shard_0(contest_id, user_id) WHERE closed_at IS NULL;
CREATE UNIQUE INDEX idx_positions_s0_position_id ON positions_shard_0(position_id);

-- Shard 1 indexes
CREATE INDEX idx_positions_s1_contest ON positions_shard_1(contest_id);
CREATE INDEX idx_positions_s1_user ON positions_shard_1(user_id);
CREATE INDEX idx_positions_s1_symbol ON positions_shard_1(symbol);
CREATE INDEX idx_positions_s1_opened_at ON positions_shard_1(opened_at);
CREATE INDEX idx_positions_s1_closed_at ON positions_shard_1(closed_at);
CREATE INDEX idx_positions_s1_contest_user ON positions_shard_1(contest_id, user_id);
CREATE INDEX idx_positions_s1_contest_user_symbol ON positions_shard_1(contest_id, user_id, symbol);
CREATE INDEX idx_positions_s1_open ON positions_shard_1(contest_id, user_id) WHERE closed_at IS NULL;
CREATE UNIQUE INDEX idx_positions_s1_position_id ON positions_shard_1(position_id);

-- Shard 2 indexes
CREATE INDEX idx_positions_s2_contest ON positions_shard_2(contest_id);
CREATE INDEX idx_positions_s2_user ON positions_shard_2(user_id);
CREATE INDEX idx_positions_s2_symbol ON positions_shard_2(symbol);
CREATE INDEX idx_positions_s2_opened_at ON positions_shard_2(opened_at);
CREATE INDEX idx_positions_s2_closed_at ON positions_shard_2(closed_at);
CREATE INDEX idx_positions_s2_contest_user ON positions_shard_2(contest_id, user_id);
CREATE INDEX idx_positions_s2_contest_user_symbol ON positions_shard_2(contest_id, user_id, symbol);
CREATE INDEX idx_positions_s2_open ON positions_shard_2(contest_id, user_id) WHERE closed_at IS NULL;
CREATE UNIQUE INDEX idx_positions_s2_position_id ON positions_shard_2(position_id);

-- Shard 3 indexes
CREATE INDEX idx_positions_s3_contest ON positions_shard_3(contest_id);
CREATE INDEX idx_positions_s3_user ON positions_shard_3(user_id);
CREATE INDEX idx_positions_s3_symbol ON positions_shard_3(symbol);
CREATE INDEX idx_positions_s3_opened_at ON positions_shard_3(opened_at);
CREATE INDEX idx_positions_s3_closed_at ON positions_shard_3(closed_at);
CREATE INDEX idx_positions_s3_contest_user ON positions_shard_3(contest_id, user_id);
CREATE INDEX idx_positions_s3_contest_user_symbol ON positions_shard_3(contest_id, user_id, symbol);
CREATE INDEX idx_positions_s3_open ON positions_shard_3(contest_id, user_id) WHERE closed_at IS NULL;
CREATE UNIQUE INDEX idx_positions_s3_position_id ON positions_shard_3(position_id);

-- ============================================================================
-- DATA MIGRATION FROM EXISTING TABLES
-- ============================================================================

-- Migrate existing orders data to shard 0
INSERT INTO orders_partitioned (
    order_id, shard_id, contest_id, user_id, symbol, side, type,
    qty, qty_filled, limit_price, stop_price, take_profit, stop_loss,
    status, created_at, updated_at
)
SELECT
    order_id,
    COALESCE(get_shard_id_for_contest(contest_id), 0) as shard_id,
    contest_id, user_id, symbol, side, type,
    qty, qty_filled, limit_price, stop_price, take_profit, stop_loss,
    status, created_at, updated_at
FROM orders;

-- Migrate existing fills data to shard 0
INSERT INTO fills_partitioned (
    fill_id, shard_id, order_id, contest_id, user_id, symbol, side,
    qty, fill_price, created_at
)
SELECT
    fill_id,
    COALESCE(get_shard_id_for_contest(contest_id), 0) as shard_id,
    order_id, contest_id, user_id, symbol, side,
    qty, fill_price, created_at
FROM fills;

-- Migrate existing positions data to shard 0
INSERT INTO positions_partitioned (
    position_id, shard_id, contest_id, user_id, symbol, side,
    qty_open, entry_price, qty_used, realized_score, opened_at, closed_at
)
SELECT
    position_id,
    COALESCE(get_shard_id_for_contest(contest_id), 0) as shard_id,
    contest_id, user_id, symbol, side,
    qty_open, entry_price, qty_used, realized_score, opened_at, closed_at
FROM positions;

-- ============================================================================
-- SWAP TABLES (ATOMIC RENAME)
-- ============================================================================

-- Drop triggers on old tables first
DROP TRIGGER IF EXISTS set_orders_updated_at ON orders;

-- Rename old tables to backup names
ALTER TABLE orders RENAME TO orders_old;
ALTER TABLE fills RENAME TO fills_old;
ALTER TABLE positions RENAME TO positions_old;

-- Rename new partitioned tables to production names
ALTER TABLE orders_partitioned RENAME TO orders;
ALTER TABLE fills_partitioned RENAME TO fills;
ALTER TABLE positions_partitioned RENAME TO positions;

-- ============================================================================
-- CREATE TRIGGER FOR UPDATED_AT ON NEW ORDERS TABLE
-- ============================================================================

CREATE TRIGGER set_orders_updated_at
    BEFORE UPDATE ON orders
    FOR EACH ROW
    EXECUTE FUNCTION trigger_set_updated_at();

-- ============================================================================
-- CREATE VIEWS FOR BACKWARD COMPATIBILITY (WITHOUT SHARD_ID)
-- ============================================================================

-- View that hides shard_id for backward-compatible queries
CREATE VIEW orders_compat AS
SELECT
    order_id, contest_id, user_id, symbol, side, type,
    qty, qty_filled, limit_price, stop_price, take_profit, stop_loss,
    status, created_at, updated_at
FROM orders;

CREATE VIEW fills_compat AS
SELECT
    fill_id, order_id, contest_id, user_id, symbol, side,
    qty, fill_price, created_at
FROM fills;

CREATE VIEW positions_compat AS
SELECT
    position_id, contest_id, user_id, symbol, side,
    qty_open, entry_price, qty_used, realized_score, opened_at, closed_at
FROM positions;

-- ============================================================================
-- FUNCTION TO CREATE NEW SHARD PARTITIONS
-- ============================================================================

CREATE OR REPLACE FUNCTION create_shard_partitions(new_shard_id INT)
RETURNS void AS $$
DECLARE
    orders_partition_name TEXT;
    fills_partition_name TEXT;
    positions_partition_name TEXT;
BEGIN
    -- Validate shard_id exists in shard_config
    IF NOT EXISTS (SELECT 1 FROM shard_config WHERE shard_id = new_shard_id) THEN
        RAISE EXCEPTION 'Shard ID % does not exist in shard_config. Add it first.', new_shard_id;
    END IF;

    -- Generate partition table names
    orders_partition_name := 'orders_shard_' || new_shard_id;
    fills_partition_name := 'fills_shard_' || new_shard_id;
    positions_partition_name := 'positions_shard_' || new_shard_id;

    -- Check if partitions already exist
    IF EXISTS (SELECT 1 FROM pg_class WHERE relname = orders_partition_name) THEN
        RAISE NOTICE 'Partition % already exists, skipping orders', orders_partition_name;
    ELSE
        -- Create orders partition
        EXECUTE format(
            'CREATE TABLE %I PARTITION OF orders FOR VALUES IN (%s)',
            orders_partition_name, new_shard_id
        );

        -- Create indexes for orders partition
        EXECUTE format('CREATE INDEX idx_orders_s%s_contest ON %I(contest_id)', new_shard_id, orders_partition_name);
        EXECUTE format('CREATE INDEX idx_orders_s%s_user ON %I(user_id)', new_shard_id, orders_partition_name);
        EXECUTE format('CREATE INDEX idx_orders_s%s_symbol ON %I(symbol)', new_shard_id, orders_partition_name);
        EXECUTE format('CREATE INDEX idx_orders_s%s_status ON %I(status)', new_shard_id, orders_partition_name);
        EXECUTE format('CREATE INDEX idx_orders_s%s_created_at ON %I(created_at)', new_shard_id, orders_partition_name);
        EXECUTE format('CREATE INDEX idx_orders_s%s_contest_user ON %I(contest_id, user_id)', new_shard_id, orders_partition_name);
        EXECUTE format('CREATE INDEX idx_orders_s%s_contest_user_status ON %I(contest_id, user_id, status)', new_shard_id, orders_partition_name);
        EXECUTE format('CREATE INDEX idx_orders_s%s_contest_symbol ON %I(contest_id, symbol)', new_shard_id, orders_partition_name);
        EXECUTE format('CREATE UNIQUE INDEX idx_orders_s%s_order_id ON %I(order_id)', new_shard_id, orders_partition_name);

        RAISE NOTICE 'Created orders partition: %', orders_partition_name;
    END IF;

    IF EXISTS (SELECT 1 FROM pg_class WHERE relname = fills_partition_name) THEN
        RAISE NOTICE 'Partition % already exists, skipping fills', fills_partition_name;
    ELSE
        -- Create fills partition
        EXECUTE format(
            'CREATE TABLE %I PARTITION OF fills FOR VALUES IN (%s)',
            fills_partition_name, new_shard_id
        );

        -- Create indexes for fills partition
        EXECUTE format('CREATE INDEX idx_fills_s%s_order ON %I(order_id)', new_shard_id, fills_partition_name);
        EXECUTE format('CREATE INDEX idx_fills_s%s_contest ON %I(contest_id)', new_shard_id, fills_partition_name);
        EXECUTE format('CREATE INDEX idx_fills_s%s_user ON %I(user_id)', new_shard_id, fills_partition_name);
        EXECUTE format('CREATE INDEX idx_fills_s%s_symbol ON %I(symbol)', new_shard_id, fills_partition_name);
        EXECUTE format('CREATE INDEX idx_fills_s%s_created_at ON %I(created_at)', new_shard_id, fills_partition_name);
        EXECUTE format('CREATE INDEX idx_fills_s%s_contest_user ON %I(contest_id, user_id)', new_shard_id, fills_partition_name);
        EXECUTE format('CREATE INDEX idx_fills_s%s_contest_symbol ON %I(contest_id, symbol)', new_shard_id, fills_partition_name);
        EXECUTE format('CREATE UNIQUE INDEX idx_fills_s%s_fill_id ON %I(fill_id)', new_shard_id, fills_partition_name);

        RAISE NOTICE 'Created fills partition: %', fills_partition_name;
    END IF;

    IF EXISTS (SELECT 1 FROM pg_class WHERE relname = positions_partition_name) THEN
        RAISE NOTICE 'Partition % already exists, skipping positions', positions_partition_name;
    ELSE
        -- Create positions partition
        EXECUTE format(
            'CREATE TABLE %I PARTITION OF positions FOR VALUES IN (%s)',
            positions_partition_name, new_shard_id
        );

        -- Create indexes for positions partition
        EXECUTE format('CREATE INDEX idx_positions_s%s_contest ON %I(contest_id)', new_shard_id, positions_partition_name);
        EXECUTE format('CREATE INDEX idx_positions_s%s_user ON %I(user_id)', new_shard_id, positions_partition_name);
        EXECUTE format('CREATE INDEX idx_positions_s%s_symbol ON %I(symbol)', new_shard_id, positions_partition_name);
        EXECUTE format('CREATE INDEX idx_positions_s%s_opened_at ON %I(opened_at)', new_shard_id, positions_partition_name);
        EXECUTE format('CREATE INDEX idx_positions_s%s_closed_at ON %I(closed_at)', new_shard_id, positions_partition_name);
        EXECUTE format('CREATE INDEX idx_positions_s%s_contest_user ON %I(contest_id, user_id)', new_shard_id, positions_partition_name);
        EXECUTE format('CREATE INDEX idx_positions_s%s_contest_user_symbol ON %I(contest_id, user_id, symbol)', new_shard_id, positions_partition_name);
        EXECUTE format('CREATE INDEX idx_positions_s%s_open ON %I(contest_id, user_id) WHERE closed_at IS NULL', new_shard_id, positions_partition_name);
        EXECUTE format('CREATE UNIQUE INDEX idx_positions_s%s_position_id ON %I(position_id)', new_shard_id, positions_partition_name);

        RAISE NOTICE 'Created positions partition: %', positions_partition_name;
    END IF;

    RAISE NOTICE 'Successfully created all partitions for shard %', new_shard_id;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- FUNCTION TO DROP SHARD PARTITIONS (WITH SAFETY CHECKS)
-- ============================================================================

CREATE OR REPLACE FUNCTION drop_shard_partitions(target_shard_id INT)
RETURNS void AS $$
DECLARE
    orders_count BIGINT;
    fills_count BIGINT;
    positions_count BIGINT;
    orders_partition_name TEXT;
    fills_partition_name TEXT;
    positions_partition_name TEXT;
BEGIN
    -- Prevent dropping shard 0 (default shard)
    IF target_shard_id = 0 THEN
        RAISE EXCEPTION 'Cannot drop shard 0 - it is the default shard';
    END IF;

    -- Generate partition table names
    orders_partition_name := 'orders_shard_' || target_shard_id;
    fills_partition_name := 'fills_shard_' || target_shard_id;
    positions_partition_name := 'positions_shard_' || target_shard_id;

    -- Check for data in partitions
    EXECUTE format('SELECT COUNT(*) FROM %I', orders_partition_name) INTO orders_count;
    EXECUTE format('SELECT COUNT(*) FROM %I', fills_partition_name) INTO fills_count;
    EXECUTE format('SELECT COUNT(*) FROM %I', positions_partition_name) INTO positions_count;

    IF orders_count > 0 OR fills_count > 0 OR positions_count > 0 THEN
        RAISE EXCEPTION 'Shard % contains data (orders: %, fills: %, positions: %). Migrate data before dropping.',
            target_shard_id, orders_count, fills_count, positions_count;
    END IF;

    -- Drop partitions
    EXECUTE format('DROP TABLE IF EXISTS %I', orders_partition_name);
    EXECUTE format('DROP TABLE IF EXISTS %I', fills_partition_name);
    EXECUTE format('DROP TABLE IF EXISTS %I', positions_partition_name);

    RAISE NOTICE 'Successfully dropped all partitions for shard %', target_shard_id;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- FUNCTION TO MIGRATE DATA BETWEEN SHARDS
-- ============================================================================

CREATE OR REPLACE FUNCTION migrate_contest_to_shard(
    p_contest_id UUID,
    p_target_shard_id INT
)
RETURNS void AS $$
DECLARE
    current_shard_id INT;
    order_count BIGINT;
    fill_count BIGINT;
    position_count BIGINT;
BEGIN
    -- Validate target shard exists
    IF NOT EXISTS (SELECT 1 FROM shard_config WHERE shard_id = p_target_shard_id) THEN
        RAISE EXCEPTION 'Target shard % does not exist in shard_config', p_target_shard_id;
    END IF;

    -- Get current shard_id for the contest
    SELECT COALESCE(shard_id, 0) INTO current_shard_id FROM contests WHERE id = p_contest_id;

    IF current_shard_id IS NULL THEN
        RAISE EXCEPTION 'Contest % not found', p_contest_id;
    END IF;

    IF current_shard_id = p_target_shard_id THEN
        RAISE NOTICE 'Contest % is already on shard %, no migration needed', p_contest_id, p_target_shard_id;
        RETURN;
    END IF;

    -- Count records to migrate
    SELECT COUNT(*) INTO order_count FROM orders WHERE contest_id = p_contest_id;
    SELECT COUNT(*) INTO fill_count FROM fills WHERE contest_id = p_contest_id;
    SELECT COUNT(*) INTO position_count FROM positions WHERE contest_id = p_contest_id;

    RAISE NOTICE 'Migrating contest % from shard % to shard %', p_contest_id, current_shard_id, p_target_shard_id;
    RAISE NOTICE 'Records to migrate - Orders: %, Fills: %, Positions: %', order_count, fill_count, position_count;

    -- Migrate orders (insert into new partition, then delete from old)
    INSERT INTO orders (
        order_id, shard_id, contest_id, user_id, symbol, side, type,
        qty, qty_filled, limit_price, stop_price, take_profit, stop_loss,
        status, created_at, updated_at
    )
    SELECT
        order_id, p_target_shard_id, contest_id, user_id, symbol, side, type,
        qty, qty_filled, limit_price, stop_price, take_profit, stop_loss,
        status, created_at, updated_at
    FROM orders
    WHERE contest_id = p_contest_id AND shard_id = current_shard_id;

    DELETE FROM orders WHERE contest_id = p_contest_id AND shard_id = current_shard_id;

    -- Migrate fills
    INSERT INTO fills (
        fill_id, shard_id, order_id, contest_id, user_id, symbol, side,
        qty, fill_price, created_at
    )
    SELECT
        fill_id, p_target_shard_id, order_id, contest_id, user_id, symbol, side,
        qty, fill_price, created_at
    FROM fills
    WHERE contest_id = p_contest_id AND shard_id = current_shard_id;

    DELETE FROM fills WHERE contest_id = p_contest_id AND shard_id = current_shard_id;

    -- Migrate positions
    INSERT INTO positions (
        position_id, shard_id, contest_id, user_id, symbol, side,
        qty_open, entry_price, qty_used, realized_score, opened_at, closed_at
    )
    SELECT
        position_id, p_target_shard_id, contest_id, user_id, symbol, side,
        qty_open, entry_price, qty_used, realized_score, opened_at, closed_at
    FROM positions
    WHERE contest_id = p_contest_id AND shard_id = current_shard_id;

    DELETE FROM positions WHERE contest_id = p_contest_id AND shard_id = current_shard_id;

    -- Update contest shard_id
    UPDATE contests SET shard_id = p_target_shard_id WHERE id = p_contest_id;

    -- Log the migration
    INSERT INTO shard_assignment_log (contest_id, old_shard_id, new_shard_id, reason)
    VALUES (p_contest_id, current_shard_id, p_target_shard_id, 'Data migration via migrate_contest_to_shard');

    RAISE NOTICE 'Successfully migrated contest % to shard %', p_contest_id, p_target_shard_id;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- ADD ADDITIONAL SHARDS TO SHARD_CONFIG (1, 2, 3)
-- ============================================================================

INSERT INTO shard_config (shard_id, name, kafka_partition, status, weight)
VALUES
    (1, 'shard-1', 1, 'active', 100),
    (2, 'shard-2', 2, 'active', 100),
    (3, 'shard-3', 3, 'active', 100)
ON CONFLICT (shard_id) DO NOTHING;

-- ============================================================================
-- STATISTICS AND MONITORING VIEWS
-- ============================================================================

-- View to see partition statistics
CREATE VIEW partition_stats AS
SELECT
    'orders' as table_name,
    child.relname as partition_name,
    pg_relation_size(child.oid) as size_bytes,
    pg_size_pretty(pg_relation_size(child.oid)) as size_pretty,
    (SELECT COUNT(*) FROM orders WHERE shard_id = CAST(SUBSTRING(child.relname FROM 'shard_(\d+)') AS INT)) as row_count
FROM pg_inherits
JOIN pg_class parent ON pg_inherits.inhparent = parent.oid
JOIN pg_class child ON pg_inherits.inhrelid = child.oid
WHERE parent.relname = 'orders'
UNION ALL
SELECT
    'fills' as table_name,
    child.relname as partition_name,
    pg_relation_size(child.oid) as size_bytes,
    pg_size_pretty(pg_relation_size(child.oid)) as size_pretty,
    (SELECT COUNT(*) FROM fills WHERE shard_id = CAST(SUBSTRING(child.relname FROM 'shard_(\d+)') AS INT)) as row_count
FROM pg_inherits
JOIN pg_class parent ON pg_inherits.inhparent = parent.oid
JOIN pg_class child ON pg_inherits.inhrelid = child.oid
WHERE parent.relname = 'fills'
UNION ALL
SELECT
    'positions' as table_name,
    child.relname as partition_name,
    pg_relation_size(child.oid) as size_bytes,
    pg_size_pretty(pg_relation_size(child.oid)) as size_pretty,
    (SELECT COUNT(*) FROM positions WHERE shard_id = CAST(SUBSTRING(child.relname FROM 'shard_(\d+)') AS INT)) as row_count
FROM pg_inherits
JOIN pg_class parent ON pg_inherits.inhparent = parent.oid
JOIN pg_class child ON pg_inherits.inhrelid = child.oid
WHERE parent.relname = 'positions';

-- View to see shard distribution
CREATE VIEW shard_distribution AS
SELECT
    s.shard_id,
    s.name as shard_name,
    s.status,
    s.weight,
    COALESCE(c.contest_count, 0) as contest_count,
    COALESCE(o.order_count, 0) as order_count,
    COALESCE(f.fill_count, 0) as fill_count,
    COALESCE(p.position_count, 0) as position_count
FROM shard_config s
LEFT JOIN (
    SELECT shard_id, COUNT(*) as contest_count
    FROM contests
    GROUP BY shard_id
) c ON s.shard_id = c.shard_id
LEFT JOIN (
    SELECT shard_id, COUNT(*) as order_count
    FROM orders
    GROUP BY shard_id
) o ON s.shard_id = o.shard_id
LEFT JOIN (
    SELECT shard_id, COUNT(*) as fill_count
    FROM fills
    GROUP BY shard_id
) f ON s.shard_id = f.shard_id
LEFT JOIN (
    SELECT shard_id, COUNT(*) as position_count
    FROM positions
    GROUP BY shard_id
) p ON s.shard_id = p.shard_id
ORDER BY s.shard_id;

-- ============================================================================
-- COMMENTS FOR DOCUMENTATION
-- ============================================================================

COMMENT ON TABLE orders IS 'Partitioned orders table by shard_id for horizontal scaling';
COMMENT ON TABLE fills IS 'Partitioned fills table by shard_id for horizontal scaling';
COMMENT ON TABLE positions IS 'Partitioned positions table by shard_id for horizontal scaling';

COMMENT ON FUNCTION create_shard_partitions(INT) IS 'Creates new partition tables for orders, fills, and positions for a given shard_id';
COMMENT ON FUNCTION drop_shard_partitions(INT) IS 'Drops empty partition tables for a given shard_id (safety check prevents dropping non-empty partitions)';
COMMENT ON FUNCTION migrate_contest_to_shard(UUID, INT) IS 'Migrates all data for a contest from current shard to target shard';
COMMENT ON FUNCTION get_shard_id_for_contest(UUID) IS 'Returns the shard_id for a given contest_id';

COMMENT ON VIEW partition_stats IS 'Shows size and row count statistics for each partition';
COMMENT ON VIEW shard_distribution IS 'Shows data distribution across shards';
COMMENT ON VIEW orders_compat IS 'Backward-compatible view of orders without shard_id';
COMMENT ON VIEW fills_compat IS 'Backward-compatible view of fills without shard_id';
COMMENT ON VIEW positions_compat IS 'Backward-compatible view of positions without shard_id';
