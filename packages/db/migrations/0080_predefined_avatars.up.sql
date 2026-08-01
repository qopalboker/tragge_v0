-- Predefined avatars table: allows admin CRUD management of avatar options
CREATE TABLE predefined_avatars (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        VARCHAR(50) NOT NULL UNIQUE,
    display_name VARCHAR(100) NOT NULL,
    category    VARCHAR(20) NOT NULL DEFAULT 'animal' CHECK (category IN ('animal', 'character', 'special')),
    bg_color    VARCHAR(7) NOT NULL DEFAULT '#2a2a3a',
    image_path  VARCHAR(500) NOT NULL,
    sort_order  INT NOT NULL DEFAULT 0,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_predefined_avatars_active ON predefined_avatars (is_active, sort_order);

-- Seed existing 16 avatars
INSERT INTO predefined_avatars (slug, display_name, category, bg_color, image_path, sort_order) VALUES
('shark',   'Business Shark', 'animal',    '#1e3a5f', '/avatars/shark.png',   1),
('monkey',  'Diamond Monkey', 'animal',    '#4a3728', '/avatars/monkey.png',  2),
('snake',   'Viper',          'animal',    '#2d4a2d', '/avatars/snake.png',   3),
('lion',    'King Lion',      'animal',    '#8b6914', '/avatars/lion.png',    4),
('dragon',  'Dragon',         'animal',    '#4a1a1a', '/avatars/dragon.png',  5),
('panda',   'Tech Panda',     'animal',    '#2d2d2d', '/avatars/panda.png',   6),
('eagle',   'Cyber Eagle',    'animal',    '#1a1a3a', '/avatars/eagle.png',   7),
('phoenix', 'Phoenix',        'special',   '#4a2a0a', '/avatars/phoenix.png', 8),
('wolf',    'Shadow Wolf',    'animal',    '#2a2a3a', '/avatars/wolf.png',    9),
('samurai', 'Samurai',        'character', '#3a1a1a', '/avatars/samurai.png', 10),
('bull',    'Bull',           'animal',    '#3a2a1a', '/avatars/bull.png',   11),
('cat',     'VR Cat',         'animal',    '#2a3a4a', '/avatars/cat.png',    12),
('bear',    'Bear',           'animal',    '#4a3a2a', '/avatars/bear.png',   13),
('fox',     'Smart Fox',      'animal',    '#5a3a1a', '/avatars/fox.png',    14),
('owl',     'Night Owl',      'animal',    '#1a2a3a', '/avatars/owl.png',    15),
('robot',   'Trading Bot',    'special',   '#2a3a4a', '/avatars/robot.png',  16);
