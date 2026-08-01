-- Migration: 0076_update_crypto_symbols (down)
-- Description: Revert crypto symbol changes

-- Re-activate the old symbols
UPDATE symbols SET is_active = TRUE, updated_at = NOW()
WHERE symbol IN ('ETC/USD', 'ARB/USD', 'OP/USD', 'INJ/USD', 'RENDER/USD');

-- Deactivate the new symbols (don't delete — they may have been used)
UPDATE symbols SET is_active = FALSE, updated_at = NOW()
WHERE symbol IN ('BCH/USD', 'CRO/USD', 'HBAR/USD', 'ICP/USD', 'VET/USD');
