-- 0002_shard_config.up.sql
-- Shard configuration for contest sharding support

-- ============================================================================
-- SHARD CONFIGURATION
-- ============================================================================

CREATE TYPE shard_status AS ENUM ('active', 'draining', 'inactive', 'maintenance');

CREATE TABLE shard_config (
    shard_id INT PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE,
    status shard_status NOT NULL DEFAULT 'active',
    weight INT NOT NULL DEFAULT 100,
    kafka_partition INT NOT NULL,
    address VARCHAR(255),
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_shard_weight_positive CHECK (weight >= 0 AND weight <= 1000),
    CONSTRAINT chk_kafka_partition_positive CHECK (kafka_partition >= 0)
);

CREATE INDEX idx_shard_config_status ON shard_config(status);
CREATE INDEX idx_shard_config_kafka_partition ON shard_config(kafka_partition);

-- ============================================================================
-- ADD SHARD_ID TO CONTESTS
-- ============================================================================

ALTER TABLE contests ADD COLUMN shard_id INT DEFAULT 0;

ALTER TABLE contests ADD CONSTRAINT fk_contests_shard
    FOREIGN KEY (shard_id) REFERENCES shard_config(shard_id);

CREATE INDEX idx_contests_shard_id ON contests(shard_id);

-- ============================================================================
-- SHARD ASSIGNMENT LOG (AUDIT)
-- ============================================================================

CREATE TABLE shard_assignment_log (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    contest_id UUID NOT NULL REFERENCES contests(id) ON DELETE CASCADE,
    old_shard_id INT,
    new_shard_id INT NOT NULL,
    reason VARCHAR(255),
    assigned_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_shard_assignment_old_shard
        FOREIGN KEY (old_shard_id) REFERENCES shard_config(shard_id),
    CONSTRAINT fk_shard_assignment_new_shard
        FOREIGN KEY (new_shard_id) REFERENCES shard_config(shard_id)
);

CREATE INDEX idx_shard_assignment_log_contest_id ON shard_assignment_log(contest_id);
CREATE INDEX idx_shard_assignment_log_old_shard_id ON shard_assignment_log(old_shard_id);
CREATE INDEX idx_shard_assignment_log_new_shard_id ON shard_assignment_log(new_shard_id);
CREATE INDEX idx_shard_assignment_log_created_at ON shard_assignment_log(created_at);
CREATE INDEX idx_shard_assignment_log_assigned_by ON shard_assignment_log(assigned_by);

-- ============================================================================
-- INSERT DEFAULT SHARD
-- ============================================================================

INSERT INTO shard_config (shard_id, name, kafka_partition) VALUES (0, 'default', 0);

-- ============================================================================
-- TRIGGER FOR UPDATED_AT ON SHARD_CONFIG
-- ============================================================================

CREATE TRIGGER set_shard_config_updated_at
    BEFORE UPDATE ON shard_config
    FOR EACH ROW
    EXECUTE FUNCTION trigger_set_updated_at();
