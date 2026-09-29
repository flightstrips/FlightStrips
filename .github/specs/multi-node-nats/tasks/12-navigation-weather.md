# Task 12 — Navigation objects and provider caches

**Depends on:** 03 and 11. **Outcome:** immutable navigation and provider checkpoints recover without PostgreSQL.

## Work

- Store typed `NavData` fragments and only typed `ProviderPage` values as checksum-verified `FS_OBJECTS` objects. Decode AIRAC.net's external response at the adapter boundary; never store raw response JSON. Keep manifests, active revision, route-cache metadata, ETag and pagination checkpoints in typed airport entities. Write/verify all objects before committing a manifest switch; preserve the prior active manifest on failure.
- Move weather cache entries to airport state and provider request quota ledger to global state. Reserve quota before external calls with a stable workflow ID. If response is uncertain, quota stays consumed and no fabricated cache entry appears.

## Done when

- Existing navigation cache, import, AIRAC.net, weather and AMAN route tests pass against the new adapters. A corrupt/missing object is never served as valid data.
- Concurrent imports cannot expose a manifest with absent blobs. Crash after quota reservation does not issue a second provider request for the same workflow.
- Restore from NATS backup yields the same active manifest digest, cache provenance and quota ledger.

**Starting points:** `backend/internal/aman/navdata`, `backend/internal/repository/postgres/navigation_cache.go`, `weather_cache.go`, and AIRAC.net checkpoint adapter.
