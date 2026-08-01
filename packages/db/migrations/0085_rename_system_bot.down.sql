-- Revert the system bot back to "Tragge Trader"
UPDATE users
SET username     = 'tragge_trader',
    display_name = 'Tragge Trader',
    email        = 'system-trader@tragge.internal',
    updated_at   = NOW()
WHERE id = '00000000-0000-0000-0000-000000000001'
  AND is_system_account = TRUE;
