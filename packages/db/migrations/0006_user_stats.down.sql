-- 0006_user_stats.down.sql
-- Rollback user statistics and score history

DROP TRIGGER IF EXISTS trigger_update_user_stats ON user_score_history;
DROP FUNCTION IF EXISTS update_user_stats();
DROP TABLE IF EXISTS user_score_history;
DROP TABLE IF EXISTS user_stats;
