# Existing surface to Protobuf contract map

This inventory prevents the implementation from treating a legacy JSON payload as an escape hatch. The current browser WebSocket actions in `frontend/src/api/models.ts` and `frontend/src/api/aman.ts` are replaced by the named `ClientCommand` cases in [wire.proto](proto/wire.proto). First-party **HTTP remains JSON** and is converted to these internal typed commands or typed projection reads at its handler boundary. The EuroScope WebSocket remains Protobuf and uses [euroscope.proto](proto/euroscope.proto). **ALB is excluded and unchanged.** The external VACS WebSocket retains the VACS provider protocol only inside its adapter. The storage families map to [storage.proto](proto/storage.proto) as listed in [state-map.md](state-map.md).

| Existing browser action | New Protobuf action |
| --- | --- |
| `token` | `FrontendFrame.authenticate` |
| `move` | `StripAction.move` |
| `generate_squawk` | `StripAction.generate_squawk` |
| `update_strip_data` | `StripAction.update_data` (all optional fields in one atomic command) |
| `update_order` | `StripAction.set_order` with typed `StripRef` |
| `send_message`, `send_private_message` | `MessageAction.broadcast`, `.private_message` |
| `cdm_ready` | `CdmAction.set_ready` (`ready=true`) |
| `release_point`, `start_req`, `marked` | `StripAction.set_release_point`, `.set_start_requested`, `.set_marked` |
| `runway_clearance`, `runway_confirmation` | `StripAction.set_runway_cleared`, `.set_runway_confirmed` (`value=true`) |
| `issue_pdc_clearance`, `revert_to_voice` | `PdcAction.issue`, `.revert_to_voice` |
| `coordination_transfer_request`, `coordination_assume_request`, `coordination_force_assume_request`, `coordination_free_request`, `coordination_cancel_transfer_request` | Corresponding `CoordinationAction` oneof cases |
| `coordination_tag_request`, `coordination_accept_tag_request` | `CoordinationAction.tag`, `.accept_tag` |
| `create_tactical_strip`, `delete_tactical_strip`, `confirm_tactical_strip`, `force_assume_tactical_strip`, `mark_tactical_strip`, `start_tactical_timer`, `move_tactical_strip` | Corresponding `TacticalAction` oneof cases; move uses typed `StripRef` |
| `acknowledge_unexpected_change`, `acknowledge_validation_status`, `clx_override_validation`, `clx_update_tobt` | `ValidationActionCommand` cases |
| `create_manual_fpl`, `create_vfr_fpl` | `FlightPlanAction.manual`, `.vfr` |
| `missed_approach` | `StripAction.missed_approach` |
| `update_runway_status` | `SessionAction.update_runway_status` |
| `stand_occupy`, `stand_vacate`, `stand_assignment_manual_request`, `stand_assignment_auto_request`, `stand_assignment_confirmed_override`, `stand_assignment_acknowledge`, `stand_block_create`, `stand_block_remove` | Corresponding `StandAction` oneof cases |
| All 30 `aman.*` actions in `AMANCommandType` | One case each in `AmanAction.change`, field numbers 2–31 |

The old `EventType` browser messages are replaced by a smaller typed publication protocol. `initial` becomes `FrontendInitial`; all domain changes (strip, controller, sector, runway, tactical, stand, PDC, CDM, AMAN, coordination, ATIS, messages, validation and policy) become `FrontendDelta` with full typed `EntityChange` records. Position and online/offline observations become `FrontendObservation`. Command rejection and final status become `FrontendActionResult`; reconnect queries use `ActionStatusQuery`/`ActionStatusMissing`. A message that was only a partial field update is no longer a wire contract; the frontend derives that presentation from the complete replacement entity. This change intentionally removes the legacy per-event JSON discriminators rather than encoding them as strings inside Protobuf.

| Previous durable family | Typed destination |
| --- | --- |
| Session, airport and ID registry | `AirportRegistry`, `SessionRegistry`, `Session` |
| Controller/sector/runway/strip/coordination | `Controller`, `SectorOwner`, `Runway`, `Strip`, `Coordination` |
| Tactical/PDC/stand/CDM/ECFMP/ATIS/CLX/message | Corresponding `EntityRecord.value` oneof cases |
| AMAN airport/flight/coordination/audit/validation/observation | `AmanAirport`, `AmanFlight`, `AmanCoordination`, `AmanAudit`, `AmanValidation`, `VatsimObservation` |
| Navigation fragments and route cache | `ObjectValue.nav` with `NavData.fragment`; `NavManifest`, `NavRouteCache` indexes |
| AIRAC.net checkpoint/page | `ProviderCheckpoint`, optional typed `ObjectValue.provider_page`; raw body discarded |
| Weather cache/provider quota | `WeatherCache`, `ProviderQuota` |
| Plugin effects/deadlines/outcomes | `EffectRecord`, `SessionDeadline`, `CommandOutcome` |
| Last aircraft position and live presence | `PositionValue`, `PresenceValue` |

No previous PostgreSQL bytes are migrated because production starts empty. Legacy compatibility fields such as AMAN `SelectedFeeder`, which exist only to read old JSON rows, are retired. Current active domain semantics and validation rules remain the source of truth for conversion into the corresponding typed fields; a conversion that cannot represent an active value fails before publish.
