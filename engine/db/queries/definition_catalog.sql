-- name: UpsertDefinitionCatalogEntry :one
INSERT INTO engine.definition_catalog (
    definition_name,
    definition_version
)
VALUES ($1, $2)
ON CONFLICT (definition_name, definition_version) DO UPDATE SET
    runtime_published_at = NOW(),
    updated_at = NOW()
RETURNING *;

-- name: GetDefinitionCatalogEntry :one
SELECT *
FROM engine.definition_catalog
WHERE definition_name = $1
  AND definition_version = $2;

-- name: ListDefinitionCatalog :many
SELECT *
FROM engine.definition_catalog
ORDER BY definition_name ASC, definition_version ASC;

-- name: TouchDefinitionCatalogEntry :execrows
UPDATE engine.definition_catalog
SET runtime_published_at = NOW(),
    updated_at = NOW()
WHERE definition_name = $1
  AND definition_version = $2;

-- name: DeleteDefinitionCatalogEntry :execrows
DELETE FROM engine.definition_catalog
WHERE definition_name = $1
  AND definition_version = $2;
