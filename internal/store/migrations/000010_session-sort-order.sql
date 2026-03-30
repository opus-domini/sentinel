ALTER TABLE sessions ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;
ALTER TABLE session_presets ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;

WITH ordered_sessions AS (
    SELECT
        name,
        ROW_NUMBER() OVER (ORDER BY name COLLATE NOCASE ASC) AS position
    FROM sessions
)
UPDATE sessions
SET sort_order = (
    SELECT position
    FROM ordered_sessions
    WHERE ordered_sessions.name = sessions.name
)
WHERE sort_order = 0;

WITH ordered_presets AS (
    SELECT
        name,
        ROW_NUMBER() OVER (ORDER BY name COLLATE NOCASE ASC) AS position
    FROM session_presets
)
UPDATE session_presets
SET sort_order = (
    SELECT position
    FROM ordered_presets
    WHERE ordered_presets.name = session_presets.name
)
WHERE sort_order = 0;
