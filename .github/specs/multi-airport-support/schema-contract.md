# Airport configuration schema v1

Status: concrete combined-file contract for coded workflow handlers, with two
validated examples and an executable isolated Go proof. Application startup has
not been migrated. This draft replaces the earlier profile/rule-language design.

See the [planning register](planning-readiness.md) for D1–D5, confirmed by the
user on 2026-11-01. Follow existing EKCH execution; allow genuine automatic
effects through the same session master or assuming controller; deploy together
while nobody is online. PDC/AMAN remain disabled at EKBI/EKYT. The schema and
fictional proof do not themselves implement these production behaviors.

## Artifacts and strictness

- [JSON Schema](schema/airport-configuration.schema.json): Draft 2020-12,
  37 reachable typed definitions, one root airport document.
- [Complete EKCH file](examples/ekch.airport.yaml): all current settings plus
  handler selection, settings, and bays, without a separate workflow profile.
- [Fictional ZZZZ file](examples/zzzz.airport.yaml): concrete second airport using
  a different handler, no ownership, and disabled services. Never deploy it.
- [Executable proof and commands](handler-design.md): actual Go handlers,
  per-session executor, tests, race checks, and dispatch benchmarks.
- [Validator](schema/validate_examples.py) and [report](schema/validation-report.json):
  structural/reference/handler checks and lossless EKCH reconstruction.

All objects are closed unless explicitly typed maps; unknown properties and
null placeholders are errors. The root requires `kind: airport`,
`schema_version: flightstrips-airport/v1`, and a four-uppercase-letter airport.
Frequencies remain strings. Paths are repository-relative and cannot escape the
repository. Duplicate YAML keys and stable IDs are rejected.

One airport file combines the operational configuration. Large service datasets
remain referenced assets; they are not copied into every YAML document. No
application-scoped VATSIM fetch/cache configuration belongs in an airport file.
The current application still loads `backend/config/ekch.yaml`.

### Airport fields

| Field | Contract |
| --- | --- |
| `$schema` | Optional editor hint pointing to the schema file. |
| `schema_version`, `kind`, `airport` | Required document identity. |
| `geography` | Required latitude/longitude/elevation, transition altitude, declared runways, runway initial-CFL map, and an optional region-file resource (required when a coverage rule names a region). Values carry explicit units in their names. |
| `positions` | Position name, string frequency, and section; names must be unique. |
| `coverage.matching` | Primary-frequency matching, tuned-frequency participation, callsign fallback, and registered normalization/provider-selection strategy. v1 uses the existing VATSIM normalization and coverage algorithm. |
| `coverage.responsibilities` | Rules with stable IDs, logical name/optional key, optional display/region/runway/direction constraints, name priority, and ordered providers. Variant rules may share a logical responsibility but cannot share a rule ID. |
| `coverage.airborne_fallback` | Ordered configured positions providing the existing airborne fallback. |
| `coverage.membership` | Either `legacy-single-airport/v1`, preserving today's association while migrating EKCH, or `remaining-responsibilities/v1` with a nonnegative `relinquish_grace_seconds`. The latter supports the agreed automatic leave/rejoin policy. The legacy label preserves EKCH association semantics; it does not require support for an old client protocol. |
| `master` | Ordered position/matching rules, higher-priority preemption flag, equal-priority policy, fallback policy, and an explicit false observer-eligibility value. |
| `ownership` | Independent strip mode (`none`/`route`) and sector mode (`none`/`coverage`), plus eligible callsign prefixes. Coverage does not require ownership to exist. |
| `routing` | Stable-ID ground routes, airborne SID families/default, and runway-to-position missed-approach mapping. Ground routes retain the current typed runway/stand selectors, path, active-runway conditions, and owner overrides. |
| `view` | A coded view ID, existing position/staffing/runway layout variants, airborne fallback layout, and message-area recipients. The ordered layout list keeps the existing last-matching-variant semantics. |
| `workflow` | Registered handler ID, closed handler-specific settings, complete bay catalog, and handler-supported typed annotation declarations in this same file. |
| `services` | A closed map of reusable services. Each entry declares availability and the service's typed config/data dependencies when active or deployment-managed. |
| `validation` | Typed clearance and taxiway validation definitions, preserving current domain formats. PDC-specific validation belongs to its service entry. |
| `provenance` | Source paths and normalized-LF SHA-256 fingerprints for this worked migration. These are audit metadata, not executable imports or authorization. |

Numeric runway selectors and numeric stand identifiers are accepted where the
existing YAML uses them. Resolution stringifies selectors in the same way as
the current loader; new authored runway strings should be quoted. Supporting
those typed scalar alternatives allows the example to reconstruct the original
parsed YAML exactly, not just drop values that are inconvenient for a new shape.

All repository resource paths are relative to the repository root, not the
including file. `repo_path` cannot escape the repository. A resource can instead
name an environment variable and a default repository path for runtime-installed
data. The ignored ICAO aircraft database is represented that way: validation
does not falsely claim the deployment asset exists in Git. Credentials and
external-service logon secrets remain deployment concerns.

For region files, semantic validation must read the selected resource to check
coverage region names. A nonblank environment override takes precedence; unset
or blank values use `default_repo_path`. Absolute environment paths may reference
deployment files outside the repository; relative overrides resolve from the
repository root. Repository paths/defaults remain constrained to the repository.
A missing selected region file is a validation error, not a fallback to another
file or a `KeyError`.

## Reusable service entries

`availability` is one of `enabled`, `disabled`, or `deployment`:

- `enabled` requests the capability at this airport, subject to deployment
  support, required credentials, and service readiness.
- `disabled` disables it at this airport; its data fields are not required.
- `deployment` preserves the current environment/application enable flag during
  migration. It does not assert that the service is currently running.

The EKCH example uses `deployment` rather than guessing the current flags.
This preserves EKCH activation; it does not turn PDC/AMAN on for EKBI/EKYT,
where both are explicitly disabled. There is no committed regional activation date.
Disabled services still reject unknown properties. An unavailable dependency
must follow the service's existing readiness behavior; it must not load EKCH
data as a fallback for another airport.

| Service | Typed configuration/data |
| --- | --- |
| `cdm` | `config_root` and the complete current CDM config: rates, LVO rate, rate/SID-interval/taxizone URIs, default taxi time, and deice durations/platforms. Existing domain field names are preserved. Relative CDM data URIs resolve below `config_root`; HTTP(S) URIs remain external resources. |
| `stand_assignment` | Stand, aircraft-reference, ICAO aircraft, airline-assignment, and scenery resources. The ICAO resource may use the existing environment override. Mutable assignments/reservations remain session state. |
| `text_pilot` | Registered `frequency-message/v1` transport/provider and coded layout-data resource. This reuses the transport; airport-specific UI/phrase behavior still needs its view/workflow code. |
| `aman` | Existing versioned terminal and boundary resources. Its own configuration format/validation remains authoritative rather than being flattened into generic transition settings. |
| `pdc` | Availability and current heavy-aircraft/runway and SID/engine validation settings. The service, provider client, transport, polling coordinator, and deployment `HOPPIE_LOGON` are shared across enabled airports. Mailbox identity and clearance state remain airport/session-aware. No airport-local credential or poller fields are accepted. |
| `ecfmp` | Per-airport availability; the shared external client remains configured at deployment scope. |

There is deliberately no `vatsim` service or poller entry under an airport.
Shared acquisition/cache configuration remains application-scoped. Airport
rules consume snapshots and produce scoped operational results.

Likewise, PDC credentials are configured once at deployment scope, not copied
into each airport file. The existing provider client accepts a separate airport
callsign for sends/polls using the same logon. No shared-service selector is
needed: every enabled `services.pdc` entry uses the application PDC service.
Removing/adding that entry must not create or destroy a separate transport.

## Coded workflow settings

`workflow` is a closed `oneOf` keyed by `handler`, with exactly two registered
branches in this proof. There is no dynamic script, arbitrary factory name,
untyped property bag, rule interpreter, or implicit fallback.

| Handler | Required settings and constraints |
| --- | --- |
| `ekch/v1` | `airborne_altitude_agl_ft`: positive integer; `relevant_distance_nm`: positive number. A nonempty `bays` list contains every existing EKCH bay ID/label. No annotation block is accepted. The adapter requires airport EKCH, existing route/coverage ownership, and the helper's legacy geography/distance. |
| `example-paper/v1` | `initial_bay`, `runway_bay`: distinct known bay IDs; `bay_ground_states`: map of known bay IDs to ES ground-state enums; `ground_state_bays`: reverse map to known bay IDs. `bays` contains unique IDs/labels. `annotations.review_required` is required and closed with `type: boolean`, `scope: session`, `default: false`. Ownership must be disabled. |

Ground-state values are `''`, `ST-UP`, `PUSH`, `TAXI`, `LINEUP`, `DEPA`, and
`PARK`. A mapping is configuration interpreted by handler code; absence of an
outgoing mapping means no ES ground-state intent. The example paper handler
uses incoming mappings only for first placement and preserves later manual
placement. Its coded `review.clear` feature has no counterpart in EKCH.

The boolean annotation is stored per strip in its airport session. It is not a
browser value, one global board flag, or a canonical ES field. Its changes use
ordinary revisioned core actions and follow observer/readiness rules. This
concrete example does not claim to define the full RADIS annotation catalog.

For another implementation, add its closed schema branch, corresponding typed
Go settings, registered factory, action contract, and tests. Same behavior with
different values reuses a handler. A new airport-specific behavior can be code
without expanding a universal workflow language. Admission/removal thresholds
belong in the settings of the handler implementing that lifecycle; no separate
lifecycle adapter document is required.

The full Python validator checks all domain sections and repository references.
The isolated Go loader strictly decodes the workflow and required context; other
domain blocks are retained as YAML nodes and not installed into existing
singletons. Production startup must compose these domain validators rather than
mistake this example projection for a full application loader.

Full-file validation, strict workflow decoding, and runtime activation are
separate steps. Production must compose the domain validators and ensure parity
with the authored schema (E1), including required fields, all legacy bays, and
supported coded views. The example Go loader is not itself proof of that parity.
Airport files are selected explicitly for deployment; examples are not discovered
and activated automatically. The sample's fixed command catalog and boolean
annotation support do not imply a completed production action/data contract (E2).

## EKCH mapping and evidence

All 20 top-level settings from the current YAML are reconstructed by the
validator and compared with the original parsed YAML, including the ordering of
arrays. Comments, numeric spelling, anchors, and whitespace are not runtime
settings and are not compared byte-for-byte.

| Existing setting(s) | New location |
| --- | --- |
| `latitude`, `longitude` | `geography.latitude`, `geography.longitude` |
| `transition_altitude`, `runway_initial_cfl`, `runways` | `geography.transition_altitude_ft`, `geography.initial_cfl_ft`, `geography.runways` |
| `positions` | `positions` unchanged |
| `sectors` | `coverage.responsibilities`; add stable rule IDs and rename `owner` to `providers`, retaining all other fields/order. `ownership.sectors: coverage` selects the compatibility projection. |
| `owner_callsign_prefixes` | `ownership.callsign_prefixes` |
| `airborne_owners` | `coverage.airborne_fallback` |
| `airborne_fallback_layout`, `layouts`, `message_areas` | Corresponding `view` fields |
| `routes` | `routing.ground`, with added stable IDs; all selectors, paths, active conditions, and overrides retained |
| `airborne_routes`, `missed_approach_handover` | `routing.airborne`, `routing.missed_approach` |
| `airborne_altitude_agl` | `workflow.settings.airborne_altitude_agl_ft` |
| `cdm` | `services.cdm.config` unchanged |
| `pdc_validation` | `services.pdc.validation` unchanged |
| `clx_validation`, `taxiway_type_validation` | `validation.clearance`, `validation.taxiway_types` unchanged |

The example contains 25 positions, 21 sector variants, 33 ground routes, two
airborne SID families, and 19 position-layout mappings. The same combined file
contains all 18 bay constants. Code-derived values are the five-position master
priority, elevation of 17 ft, and relevance distance of 30 NM. Their source
fingerprints are included so stale examples are detected on revalidation.

This validates representability and a lossless settings migration. It does not
prove full runtime behavior equivalence: automatic bay reconciliation, service
lifecycles, frontend actions, and commands still need handler/core
integration and controlled integration checks in the implementation work.
Existing backend configuration, service flags, and deployment behavior are
unchanged by this example.

## Reproduce the checks

From the repository root:

```powershell
python -m pip install -r .github/specs/multi-airport-support/schema/requirements.txt
python .github/specs/multi-airport-support/schema/validate_examples.py --write-report
```

An isolated package directory can be supplied with `--dependency-path PATH`.
`--refresh-ekch --write-report` regenerates only the combined EKCH example from
current source data; it does not alter the fictional example or runtime YAML.

The report records two validated complete airport documents and 34 rejected
invalid cases, including unknown/cross-handler settings, missing bays, invalid
mappings/annotation types, ownership incompatibility, scripts/profile references,
airport-local PDC logon/poller fields,
missing selected region override/fallback files,
unknown positions/routes, invalid resource paths, and session-owned feed caches.
See the [handler guide](handler-design.md) for runnable demonstrations, race
checks, and benchmarks. Settings preservation and the tested handler decisions
are separate claims; complete runtime parity remains integration work.
