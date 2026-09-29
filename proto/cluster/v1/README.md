# Cluster schema tooling

`storage.proto` and `wire.proto` are byte-for-byte copies of the normative
candidates in `.github/specs/multi-node-nats/proto/`. The revision-2 EuroScope
schema is installed at `proto/euroscope.proto` with generated Go and C++ bindings.

Install `protoc` 26.1, `protoc-gen-go` 1.36.11, Python requirements from
`scripts/cluster-proto-requirements.txt`, and run `npm ci` in `frontend/`.
Then, from the repository root, generate all three languages with:

```sh
python scripts/cluster_proto.py
```

Use `python scripts/cluster_proto.py --check` to compile descriptors, check
the committed compatibility baseline, run binary oneof and optional-zero
fixtures, lint the schema, and compare generated files. Run
`python scripts/check_cluster_contract.py` to check legacy surface coverage
and JSON use in the new owned binary paths. A reviewed breaking schema
change may update `baseline.pb` with `--update-baseline` after the normative
contracts and affected task checks have been updated.

The generated Go package includes `UnmarshalStrict`, which rejects malformed
wire data, unknown fields, and unknown enum values recursively. Owners still
must validate domain identities, required oneofs, timestamps, and state
transitions after decoding.
