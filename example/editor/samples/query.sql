-- A portable query with named parameters.
SELECT d.id, d.title, COUNT(t.name) AS tag_count
FROM documents AS d
LEFT JOIN tags AS t ON t.document_id = d.id
WHERE d.editable = TRUE AND d.title LIKE :pattern
GROUP BY d.id, d.title
ORDER BY tag_count DESC, d.title ASC
LIMIT 20;

/* Strings escape quotes by doubling them. */
INSERT INTO documents (title, editable)
VALUES ('Editor''s notes', TRUE);
