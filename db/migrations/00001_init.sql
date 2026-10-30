-- goose Up

-- 1. Accounts
CREATE TABLE accounts (
    id             BIGSERIAL PRIMARY KEY,
    name           TEXT NOT NULL UNIQUE,             -- e.g. collector:ondc_settlement, receiver:operative
    type           TEXT NOT NULL CHECK (type IN ('asset', 'liability', 'equity', 'income', 'expense', 'external')),
    currency       TEXT NOT NULL DEFAULT 'INR',
    constrained    BOOLEAN NOT NULL DEFAULT FALSE,  -- true = no-negative balance enforced
    normal_balance TEXT NOT NULL CHECK (normal_balance IN ('debit', 'credit')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_accounts_name ON accounts (name);

-- 1b. Account Balances (atomic, row-locked, lock-free balance enforcement)
CREATE TABLE account_balances (
    account_id    BIGINT PRIMARY KEY REFERENCES accounts(id),
    constrained   BOOLEAN NOT NULL,
    balance_paise BIGINT  NOT NULL DEFAULT 0,
    CONSTRAINT constrained_non_negative CHECK (NOT constrained OR balance_paise >= 0)
);

-- 2. Journal Transactions (one per atomic posting event)
CREATE TABLE journal_transactions (
    id              BIGSERIAL PRIMARY KEY,
    idempotency_key TEXT NOT NULL UNIQUE,
    description     TEXT,
    cause_type      TEXT NOT NULL,                  -- claim | instruction | bank_message | adjustment
    cause_id        BIGINT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_journal_tx_cause ON journal_transactions (cause_type, cause_id);

-- 3. Journal Entries (two or more per transaction: debits == credits)
CREATE TABLE journal_entries (
    id              BIGSERIAL,
    transaction_id  BIGINT NOT NULL,
    account_id      BIGINT NOT NULL,
    direction       TEXT NOT NULL CHECK (direction IN ('debit', 'credit')),
    amount_paise    BIGINT NOT NULL CHECK (amount_paise > 0),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Initial monthly partitions for 2026/2027
CREATE TABLE journal_entries_2026_10 PARTITION OF journal_entries
    FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');

CREATE TABLE journal_entries_2026_11 PARTITION OF journal_entries
    FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');

CREATE TABLE journal_entries_2026_12 PARTITION OF journal_entries
    FOR VALUES FROM ('2026-12-01') TO ('2027-01-01');

-- Default partition prevents 2027 time bomb cliff (S2)
CREATE TABLE journal_entries_default PARTITION OF journal_entries DEFAULT;

CREATE INDEX idx_journal_entries_account ON journal_entries (account_id, created_at);
CREATE INDEX idx_journal_entries_tx ON journal_entries (transaction_id);

-- 4. Idempotency Keys (insert-first, unique)
CREATE TABLE idempotency_keys (
    key          TEXT PRIMARY KEY,
    request_hash TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'done' CHECK (status IN ('in_progress', 'done', 'error')),
    response     JSONB,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at   TIMESTAMPTZ NOT NULL DEFAULT (NOW() + INTERVAL '7 days')
);

CREATE INDEX idx_idempotency_keys_expires ON idempotency_keys (expires_at);

-- 5. Claims (/settle submissions)
CREATE TABLE claims (
    id              BIGSERIAL PRIMARY KEY,
    message_id      TEXT NOT NULL UNIQUE,
    np_pair_id      TEXT NOT NULL,
    settlement_date DATE NOT NULL,
    submitted_by    TEXT NOT NULL CHECK (submitted_by IN ('buyer_app', 'seller_app', 'logistics_app')),
    status          TEXT NOT NULL DEFAULT 'received' CHECK (status IN ('received', 'valid', 'invalid', 'processed')),
    raw_payload     JSONB NOT NULL,
    received_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    spec_version    TEXT NOT NULL DEFAULT 'RSF-2.0'
);

CREATE INDEX idx_claims_np_date ON claims (np_pair_id, settlement_date, status);

-- 6. Claim Lines (per-order lines within a claim)
CREATE TABLE claim_lines (
    id               BIGSERIAL PRIMARY KEY,
    claim_id         BIGINT NOT NULL REFERENCES claims(id) ON DELETE RESTRICT,
    network_order_id TEXT NOT NULL,
    amount_paise     BIGINT NOT NULL CHECK (amount_paise > 0),
    commission_paise BIGINT NOT NULL DEFAULT 0 CHECK (commission_paise >= 0),
    bff_paise        BIGINT NOT NULL DEFAULT 0 CHECK (bff_paise >= 0)
);

CREATE INDEX idx_claim_lines_claim ON claim_lines (claim_id);
CREATE INDEX idx_claim_lines_order ON claim_lines (network_order_id);

-- 7. Batch Runs (non-partitioned table for global run_id uniqueness - S1)
CREATE TABLE batch_runs (
    run_id     TEXT PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 7b. Settlement Batches (partitioned by settlement_date - S1)
CREATE TABLE settlement_batches (
    id              BIGSERIAL,
    np_pair_id      TEXT NOT NULL,
    settlement_date DATE NOT NULL,
    run_id          TEXT NOT NULL REFERENCES batch_runs(run_id),
    status          TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'matching', 'balance_check', 'instructed', 'settled', 'failed')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ,
    PRIMARY KEY (id, settlement_date),
    UNIQUE (run_id, settlement_date)
) PARTITION BY RANGE (settlement_date);

CREATE TABLE settlement_batches_2026 PARTITION OF settlement_batches
    FOR VALUES FROM ('2026-01-01') TO ('2027-01-01');

CREATE TABLE settlement_batches_2027 PARTITION OF settlement_batches
    FOR VALUES FROM ('2027-01-01') TO ('2028-01-01');

CREATE TABLE settlement_batches_default PARTITION OF settlement_batches DEFAULT;

CREATE INDEX idx_batches_open ON settlement_batches (np_pair_id, settlement_date)
    WHERE status = 'open';

-- 8. Match Results
CREATE TABLE match_results (
    id               BIGSERIAL PRIMARY KEY,
    batch_id         BIGINT NOT NULL,
    settlement_date  DATE NOT NULL,
    network_order_id TEXT NOT NULL,
    buyer_claim_id   BIGINT REFERENCES claims(id),
    seller_claim_id  BIGINT REFERENCES claims(id),
    status           TEXT NOT NULL CHECK (status IN ('matched', 'unmatched', 'rejected')),
    policy_version   TEXT NOT NULL,
    matched_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_match_batch ON match_results (batch_id);
CREATE INDEX idx_match_order ON match_results (network_order_id);

-- 9. Instructions (Net Bank Instructions)
CREATE TABLE instructions (
    id              BIGSERIAL PRIMARY KEY,
    batch_id        BIGINT NOT NULL,
    from_account    TEXT NOT NULL,
    to_account      TEXT NOT NULL,
    amount_paise    BIGINT NOT NULL CHECK (amount_paise > 0),
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'credited', 'failed')),
    sent_at         TIMESTAMPTZ,
    settled_at      TIMESTAMPTZ
);

CREATE INDEX idx_instructions_batch ON instructions (batch_id);

-- 10. Batch Merkle Roots (Tamper Evidence per batch)
CREATE TABLE batch_roots (
    id             BIGSERIAL PRIMARY KEY,
    batch_id       BIGINT NOT NULL UNIQUE,
    root_hash      TEXT NOT NULL,
    prev_root_hash TEXT,
    np_pair_id     TEXT NOT NULL,
    computed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 11. Transactional Outbox (SKIP LOCKED delivery)
CREATE TABLE outbox (
    id              BIGSERIAL PRIMARY KEY,
    event_type      TEXT NOT NULL,
    payload         JSONB NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'done', 'failed')),
    attempts        INT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_until    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at    TIMESTAMPTZ
);

CREATE INDEX idx_outbox_pending ON outbox (next_attempt_at) WHERE status = 'pending';

-- 12. Indian Bank Holiday Calendar (RBI list)
CREATE TABLE calendar_days (
    date            DATE PRIMARY KEY,
    is_bank_holiday BOOLEAN NOT NULL DEFAULT FALSE,
    description     TEXT
);

-- 13. Audit Log (append-only)
CREATE TABLE audit_log (
    id          BIGSERIAL PRIMARY KEY,
    actor       TEXT NOT NULL,
    action      TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id   TEXT NOT NULL,
    payload     JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_log_entity ON audit_log (entity_type, entity_id);

-- 14. Immutability Triggers (Prevent UPDATE / DELETE on core ledger tables)
CREATE OR REPLACE FUNCTION prevent_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'Direct mutation of % is prohibited. Use reversing entries.', TG_TABLE_NAME;
END;
$$;

CREATE TRIGGER no_update_journal_entries
    BEFORE UPDATE OR DELETE ON journal_entries
    FOR EACH ROW EXECUTE FUNCTION prevent_mutation();

CREATE TRIGGER no_update_journal_transactions
    BEFORE UPDATE OR DELETE ON journal_transactions
    FOR EACH ROW EXECUTE FUNCTION prevent_mutation();

CREATE TRIGGER no_update_audit_log
    BEFORE UPDATE OR DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION prevent_mutation();

-- 15. TRUNCATE Guards (S4 & Appendix B.3)
CREATE OR REPLACE FUNCTION no_truncate() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'TRUNCATE of % prohibited', TG_TABLE_NAME;
END;
$$;

CREATE TRIGGER journal_entries_no_truncate
    BEFORE TRUNCATE ON journal_entries
    FOR EACH STATEMENT EXECUTE FUNCTION no_truncate();

CREATE TRIGGER journal_transactions_no_truncate
    BEFORE TRUNCATE ON journal_transactions
    FOR EACH STATEMENT EXECUTE FUNCTION no_truncate();

-- 16. Database Double-Entry Balance Assertion (S3 & Appendix B.3)
CREATE OR REPLACE FUNCTION assert_balanced() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    d bigint;
    c bigint;
BEGIN
    SELECT COALESCE(SUM(amount_paise) FILTER (WHERE direction='debit'), 0),
           COALESCE(SUM(amount_paise) FILTER (WHERE direction='credit'), 0)
      INTO d, c
      FROM journal_entries
     WHERE transaction_id = NEW.transaction_id;
    IF d <> c THEN
        RAISE EXCEPTION 'unbalanced journal transaction %: debit % credit %', NEW.transaction_id, d, c;
    END IF;
    RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER journal_entries_balanced
    AFTER INSERT ON journal_entries
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION assert_balanced();

-- goose Down
DROP TRIGGER IF EXISTS journal_entries_balanced ON journal_entries;
DROP FUNCTION IF EXISTS assert_balanced CASCADE;
DROP TRIGGER IF EXISTS journal_transactions_no_truncate ON journal_transactions;
DROP TRIGGER IF EXISTS journal_entries_no_truncate ON journal_entries;
DROP FUNCTION IF EXISTS no_truncate CASCADE;
DROP TRIGGER IF EXISTS no_update_audit_log ON audit_log;
DROP TRIGGER IF EXISTS no_update_journal_transactions ON journal_transactions;
DROP TRIGGER IF EXISTS no_update_journal_entries ON journal_entries;
DROP FUNCTION IF EXISTS prevent_mutation CASCADE;

DROP TABLE IF EXISTS audit_log, calendar_days, outbox, batch_roots, instructions,
    match_results, settlement_batches, settlement_batches_default, settlement_batches_2027, settlement_batches_2026,
    batch_runs, claim_lines, claims, idempotency_keys, journal_entries, journal_entries_default, journal_entries_2026_12,
    journal_entries_2026_11, journal_entries_2026_10, journal_transactions, account_balances, accounts CASCADE;
