-- name: ListAMANNavigationManifestRevisions :many
SELECT airport, revision
FROM aman_nav_manifests
WHERE airport = $1
ORDER BY revision DESC;
