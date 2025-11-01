-- name: GetSectorOwners :many
SELECT * FROM sector_owners
WHERE session = $1;

-- name: RemoveSectorOwners :exec
DELETE FROM sector_owners WHERE session = $1;

-- name: InsertSectorOwners :copyfrom
INSERT INTO sector_owners (session, sector, position, identifier)
VALUES ($1, $2, $3, $4);
