"""Check migration coverage and forbid JSON in owned binary transport paths."""

from __future__ import annotations

import pathlib
import re
import sys


ROOT = pathlib.Path(__file__).resolve().parents[1]
SPEC = ROOT / ".github/specs/multi-node-nats"


def check_coverage() -> None:
    coverage = (SPEC / "coverage.md").read_text(encoding="utf-8")
    models = (ROOT / "frontend/src/api/models.ts").read_text(encoding="utf-8")
    aman = (ROOT / "frontend/src/api/aman.ts").read_text(encoding="utf-8")
    wire = (ROOT / "proto/cluster/v1/wire.proto").read_text(encoding="utf-8")
    current_euro = (ROOT / "proto/euroscope.proto").read_text(encoding="utf-8")
    proposed_euro = (SPEC / "proto/euroscope.proto").read_text(encoding="utf-8")

    actions = re.search(r"export enum ActionType \{(.*?)\}", models, re.S).group(1)
    for action in re.findall(r'= "([^"]+)"', actions):
        if f"`{action}`" not in coverage:
            raise RuntimeError(f"unmapped frontend action: {action}")

    aman_actions = re.search(r"export type AMANCommandType =(.*?);", aman, re.S).group(1)
    active_aman = re.findall(r'"aman\.([^"]+)"', aman_actions)
    schema_aman = re.search(r"message AmanAction \{.*?oneof change \{(.*?)\}", wire, re.S).group(1)
    cases = [(name, int(number)) for name, number in re.findall(r"\b\w+\s+(\w+)\s*=\s*(\d+);", schema_aman)]
    if (len(active_aman) != 30 or len(cases) != 30 or
            {n for _, n in cases} != set(range(2, 32)) or
            {name for name, _ in cases} != set(active_aman)):
        raise RuntimeError("AMAN actions no longer match the 30 cases in coverage.md")
    if "All 30 `aman.*` actions" not in coverage:
        raise RuntimeError("AMAN mapping missing from coverage.md")

    def euro_cases(source: str) -> dict[int, str]:
        body = re.search(r"message Envelope \{.*?oneof event \{(.*?)\}", source, re.S).group(1)
        return {int(number): name for name, number in re.findall(r"\b\w+\s+(\w+)\s*=\s*(\d+);", body)}

    current, proposed = euro_cases(current_euro), euro_cases(proposed_euro)
    if any(proposed.get(number) != name for number, name in current.items() if number <= 52):
        raise RuntimeError("candidate EuroScope schema changes an active case 1–52")
    if not {53, 54} <= proposed.keys():
        raise RuntimeError("candidate EuroScope result cases missing")

    storage = (ROOT / "proto/cluster/v1/storage.proto").read_text(encoding="utf-8")
    for family in ("Session", "Controller", "SectorOwner", "Strip", "Coordination",
                   "TacticalStrip", "PdcSequence", "StandAssignment", "StandBlock",
                   "CdmState", "EcfmpState", "Atis", "ClxOverride", "AmanAirport",
                   "AmanFlight", "AmanCoordination", "AmanAudit", "AmanValidation",
                   "VatsimObservation", "NavManifest", "NavRouteCache", "ProviderCheckpoint",
                   "WeatherCache", "ProviderQuota", "EffectRecord", "SessionDeadline",
                   "CommandOutcome", "PositionValue", "PresenceValue"):
        if not re.search(rf"\bmessage {family}\b", storage):
            raise RuntimeError(f"missing durable family: {family}")
    if "Previous durable family" not in coverage:
        raise RuntimeError("durable family map missing from coverage.md")
    state_map = (SPEC / "state-map.md").read_text(encoding="utf-8")
    for migration in (ROOT / "backend/migrations").glob("*.sql"):
        source = migration.read_text(encoding="utf-8")
        # Ignore SQL comment lines, including the never-active PDC draft.
        source = "\n".join(line for line in source.splitlines()
                           if not line.lstrip().startswith("--"))
        for table in re.findall(r"CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?(\w+)", source, re.I):
            if not re.search(rf"\b{re.escape(table)}\b", state_map):
                raise RuntimeError(f"SQL family missing from state-map.md: {table}")


def check_json() -> None:
    # These are the owned binary transport/storage directories. HTTP and
    # external provider/OIDC adapters live outside them; ALB is excluded.
    owned = ("backend/internal/cluster", "backend/pkg/events/cluster",
             "frontend/src/api/cluster", "euroscope-plugin/src/plugin/websocket/cluster")
    forbidden = re.compile(r"json\.Marshal|json\.RawMessage|JSON\.stringify|protojson\.|toJson\(|fromJson\(")
    for relative in owned:
        directory = ROOT / relative
        if not directory.exists():
            continue
        for path in directory.rglob("*"):
            if not path.is_file() or path.name.endswith((".pb.go", "_pb.ts", ".pb.cc", ".pb.h")):
                continue
            if path.suffix not in {".go", ".ts", ".tsx", ".cc", ".cpp", ".h"}:
                continue
            if forbidden.search(path.read_text(encoding="utf-8")):
                raise RuntimeError(f"JSON serialization in binary path: {path}")


if __name__ == "__main__":
    try:
        check_coverage()
        check_json()
    except RuntimeError as error:
        sys.exit(str(error))
    print("coverage and binary-path JSON checks passed")
