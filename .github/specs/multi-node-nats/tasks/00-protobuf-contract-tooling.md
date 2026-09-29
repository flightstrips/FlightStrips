# Task 00 — Install the frozen Protobuf contract and generation checks

**Depends on:** none. **Outcome:** one generated, compile-checked source of truth before any NATS storage or new client code is implemented.

## Work

- Move the normative candidates in `../proto/storage.proto` and `../proto/wire.proto` into root `proto/cluster/v1/`, preserving package names and field numbers. Replace root `proto/euroscope.proto` with `../proto/euroscope.proto` only when the matching plugin/backend code is updated in task 14; until then validate that candidate independently. Generated code must never be hand-edited. Leave ALB code and protocol untouched.
- Generate Go with `protoc-gen-go` v1.36.11 (matching `backend/go.mod`), TypeScript with `@bufbuild/protoc-gen-es` 2.14.0 and `@bufbuild/protobuf` 2.14.0, and C++ with `protoc` 26.1 (matching the plugin's C++ runtime tag). Pin these exact versions in tooling/lockfiles, check generated sources in, and add one reproducible generation command. [Protobuf-ES](https://github.com/bufbuild/protobuf-es) provides binary encode/decode and generated oneof types.
- Add CI descriptor compilation, generated-file diff check, schema breaking check against the committed baseline, and a linter that rejects `Any`, `Struct`, `Value`, map fields, and all `bytes` fields except the two `EffectSecret` fields. Scan the new NATS/Object Store and frontend/EuroScope WebSocket paths for `json.Marshal`, `json.RawMessage`, `JSON.stringify`, and Protobuf JSON mapping; exclude unchanged ALB and explicit first-party HTTP/vendor/OIDC boundary adapters. Give every schema oneof case a binary round-trip fixture and test optional/zero distinction.
- Verify [coverage.md](../coverage.md) mechanically against current frontend and EuroScope WebSocket actions, durable AMAN fields and former SQL families. First-party HTTP handlers remain JSON and map their mutations into the same typed internal commands. ALB is excluded. This is a verification gate, not a license to invent a payload format later. A discovered omission changes the normative schema and contract documents before implementation of that feature.

## Done when

- The three candidate schemas compile together, generated Go/TS/C++ build in their respective projects, and generated diffs are empty on a second run.
- No new NATS, Object Store, frontend or EuroScope WebSocket value can accept JSON or an untyped payload. HTTP JSON is converted at its boundary; ALB is unchanged. A fixture for every oneof case round-trips, malformed/unknown fields fail, and the coverage manifest has no unmapped active surface.
- All later tasks use generated types and refer to [contracts.md](../contracts.md) and [storage-contract.md](../storage-contract.md) for fixed semantics.

**Starting points:** `proto/euroscope.proto`, `backend/pkg/events/euroscope`, `frontend/src/api/models.ts`, `frontend/src/api/aman.ts`, and `euroscope-plugin/CMakeLists.txt`.
