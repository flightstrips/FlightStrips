# AMAN navigation contracts

`navdata` is the provider-independent boundary for acquiring and consuming
canonical navigation geometry. A materializer composes `CycleSource`,
`AirportSource`, `ProcedureSource`, `FixSource`, and `RouteResolver`, then
writes a canonical cache. Runtime prediction and sequencing receive only the
cache-only `GeometryReader`.

The local `fixture` package proves source replacement, including EKCH SID,
STAR, approach, HA/HF/HM, unsupported-vector, and DCT route fixtures. Shared
checks are in `contracttest`; the AIRAC.NET adapter equivalence proof is owned
by #310 and is deliberately not implemented in this package.
