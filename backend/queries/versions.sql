-- name: GetAllDatabaseVersions :many
SELECT id, name FROM versions;

-- name: InsertDatabaseVersion :exec
INSERT INTO versions (id, name)
VALUES($1, $2);
