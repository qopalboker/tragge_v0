-- 0035_recreate_positions_compat.up.sql
-- Recreate positions_compat view that was dropped in migration 0032

CREATE VIEW positions_compat AS
SELECT position_id, contest_id, user_id, symbol, side, qty_open,
       entry_price, qty_used, realized_score, opened_at, closed_at
FROM positions;
