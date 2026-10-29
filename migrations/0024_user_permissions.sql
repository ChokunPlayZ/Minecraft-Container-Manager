-- Preserve existing access; new API accounts explicitly default to viewer.
ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'administrator' CHECK (role IN ('administrator','operator','viewer','custom'));
ALTER TABLE users ADD COLUMN permissions TEXT NOT NULL DEFAULT '[]';
ALTER TABLE users ADD COLUMN all_servers INTEGER NOT NULL DEFAULT 1 CHECK (all_servers IN (0,1));
ALTER TABLE users ADD COLUMN server_ids TEXT NOT NULL DEFAULT '[]';
ALTER TABLE users ADD COLUMN disabled INTEGER NOT NULL DEFAULT 0 CHECK (disabled IN (0,1));

-- These guards execute inside the write itself, including concurrent requests.
CREATE TRIGGER protect_last_administrator_update BEFORE UPDATE OF role, disabled ON users
WHEN OLD.role = 'administrator' AND OLD.disabled = 0
 AND (NEW.role <> 'administrator' OR NEW.disabled <> 0)
 AND NOT EXISTS (SELECT 1 FROM users WHERE id <> OLD.id AND role = 'administrator' AND disabled = 0)
BEGIN SELECT RAISE(ABORT, 'last active administrator'); END;

CREATE TRIGGER protect_last_administrator_delete BEFORE DELETE ON users
WHEN OLD.role = 'administrator' AND OLD.disabled = 0
 AND NOT EXISTS (SELECT 1 FROM users WHERE id <> OLD.id AND role = 'administrator' AND disabled = 0)
BEGIN SELECT RAISE(ABORT, 'last active administrator'); END;
