BEGIN;

ALTER TABLE comments
ADD COLUMN replies_count INTEGER NOT NULL DEFAULT 0 CHECK (replies_count >= 0);

-- Заполняем счётчики для уже существующих комментариев.
UPDATE comments AS parent
SET replies_count = (
    SELECT COUNT(*)
    FROM comments AS child
    WHERE child.post_id = parent.post_id AND child.parent_id = parent.id
);

COMMIT;
