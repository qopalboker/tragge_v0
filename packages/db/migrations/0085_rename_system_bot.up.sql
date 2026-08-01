-- Rebrand the system bot from "Tragge Trader" to "T-bot"
UPDATE users
SET username     = 't-bot',
    display_name = 'T-bot',
    email        = 'tbot@tragge.internal',
    updated_at   = NOW()
WHERE id = '00000000-0000-0000-0000-000000000001'
  AND is_system_account = TRUE;
