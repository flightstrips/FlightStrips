"""Generate and verify the frozen cluster Protobuf contract.

Requires protoc 26.1, protoc-gen-go 1.36.11, frontend/npm ci, and
`python -m pip install -r scripts/cluster-proto-requirements.txt`.
"""

from __future__ import annotations

import argparse
import json
import pathlib
import shutil
import subprocess
import sys
import tempfile

from google.protobuf import descriptor_pb2, descriptor_pool, message_factory
from google.protobuf.compiler import plugin_pb2


ROOT = pathlib.Path(__file__).resolve().parents[1]
SCHEMA = ROOT / "proto/cluster/v1"
CANDIDATE = ROOT / ".github/specs/multi-node-nats/proto"
BASELINE = ROOT / "proto/cluster/v1/baseline.pb"
GO_OUT = ROOT / "backend/pkg/events/cluster"
TS_OUT = ROOT / "frontend/src/api/generated/cluster/v1"
CPP_OUT = ROOT / "euroscope-plugin/src/plugin/websocket/generated/cluster/v1"
FORBIDDEN = {"google.protobuf.Any", "google.protobuf.Struct", "google.protobuf.Value"}


def run(*args: str, cwd: pathlib.Path = ROOT) -> None:
    subprocess.run(args, cwd=cwd, check=True)


def version(exe: str, expected: str, argument: str = "--version") -> None:
    actual = subprocess.check_output([exe, argument], text=True).strip()
    if expected not in actual:
        raise RuntimeError(f"{exe}: expected {expected}, got {actual}")


def descriptor(path: pathlib.Path) -> descriptor_pb2.FileDescriptorSet:
    run(
        "protoc", f"-I{SCHEMA}", f"-I{CANDIDATE}",
        "--include_imports", f"--descriptor_set_out={path}",
        "storage.proto", "wire.proto", "euroscope.proto",
        cwd=SCHEMA,
    )
    result = descriptor_pb2.FileDescriptorSet()
    result.ParseFromString(path.read_bytes())
    return result


def walk(messages, prefix=""):
    for message in messages:
        name = f"{prefix}.{message.name}" if prefix else message.name
        yield name, message
        yield from walk(message.nested_type, name)


def lint(files: descriptor_pb2.FileDescriptorSet) -> None:
    errors = []
    for file in files.file:
        if file.name not in {"storage.proto", "wire.proto", "euroscope.proto"}:
            continue
        for name, message in walk(file.message_type):
            for field in message.field:
                full = f"{file.name}:{name}.{field.name}"
                if field.type_name.lstrip(".") in FORBIDDEN:
                    errors.append(f"{full}: untyped well-known type")
                if field.type == field.TYPE_BYTES and not (
                    name == "EffectSecret" and field.name in {"ciphertext", "nonce"}
                ):
                    errors.append(f"{full}: forbidden bytes")
                if field.type == field.TYPE_MESSAGE and field.type_name.endswith("Entry"):
                    # A map entry may be nested under a message; inspect its descriptor.
                    types = {f".{file.package}.{n}": m for n, m in walk(file.message_type)}
                    if types.get(field.type_name, descriptor_pb2.DescriptorProto()).options.map_entry:
                        errors.append(f"{full}: map field")
    if errors:
        raise RuntimeError("\n".join(errors))


def symbols(files: descriptor_pb2.FileDescriptorSet) -> dict[str, tuple]:
    result = {}
    for file in files.file:
        if file.name not in {"storage.proto", "wire.proto", "euroscope.proto"}:
            continue
        for name, message in walk(file.message_type):
            result[f"{file.package}.{name}"] = tuple(sorted(
                (f.number, f.name, f.type, f.type_name, f.label,
                 message.oneof_decl[f.oneof_index].name if f.HasField("oneof_index") else "")
                for f in message.field
            ))
    return result


def breaking(current: descriptor_pb2.FileDescriptorSet) -> None:
    if not BASELINE.exists():
        raise RuntimeError("missing committed descriptor baseline")
    baseline = descriptor_pb2.FileDescriptorSet()
    baseline.ParseFromString(BASELINE.read_bytes())
    old, new = symbols(baseline), symbols(current)
    for message, fields in old.items():
        if message not in new:
            raise RuntimeError(f"removed message {message}")
        current_fields = {field[0]: field for field in new[message]}
        for field in fields:
            if current_fields.get(field[0]) != field:
                raise RuntimeError(f"changed or removed {message} field {field[0]}")
    # Enum number/name stability also matters, including nested enums.
    def enums(files):
        result = {}
        for file in files.file:
            if file.name not in {"storage.proto", "wire.proto", "euroscope.proto"}:
                continue
            for enum in file.enum_type:
                result[f"{file.package}.{enum.name}"] = {(v.number, v.name) for v in enum.value}
            for name, message in walk(file.message_type):
                for enum in message.enum_type:
                    result[f"{file.package}.{name}.{enum.name}"] = {(v.number, v.name) for v in enum.value}
        return result
    for name, values in enums(baseline).items():
        if not values <= enums(current).get(name, set()):
            raise RuntimeError(f"changed or removed enum values in {name}")


def roundtrip_oneofs(files: descriptor_pb2.FileDescriptorSet) -> None:
    """Exercise every candidate schema oneof case, including EuroScope v2."""
    pool = descriptor_pool.DescriptorPool()
    for file in files.file:
        pool.AddSerializedFile(file.SerializeToString())
    count = 0
    optionals = 0
    for file in files.file:
        if file.name not in {"storage.proto", "wire.proto", "euroscope.proto"}:
            continue
        for name, message in walk(file.message_type):
            descriptor = pool.FindMessageTypeByName(f"{file.package}.{name}")
            cls = message_factory.GetMessageClass(descriptor)
            for oneof in descriptor.oneofs:
                if oneof.name.startswith("_") and len(oneof.fields) == 1:
                    continue  # proto3 optional's synthetic oneof
                for field in oneof.fields:
                    value = cls()
                    if field.type == field.TYPE_MESSAGE:
                        getattr(value, field.name).SetInParent()
                    elif field.type == field.TYPE_STRING:
                        setattr(value, field.name, "fixture")
                    elif field.type == field.TYPE_BOOL:
                        setattr(value, field.name, True)
                    elif field.type == field.TYPE_ENUM:
                        setattr(value, field.name, field.enum_type.values[0].number)
                    else:
                        setattr(value, field.name, 1)
                    decoded = cls.FromString(value.SerializeToString())
                    if decoded.WhichOneof(oneof.name) != field.name or decoded != value:
                        raise RuntimeError(f"oneof fixture failed: {descriptor.full_name}.{field.name}")
                    count += 1
            for field in message.field:
                if not field.proto3_optional:
                    continue
                value = cls()
                if field.type == field.TYPE_MESSAGE:
                    getattr(value, field.name).SetInParent()
                elif field.type == field.TYPE_STRING:
                    setattr(value, field.name, "")
                elif field.type == field.TYPE_BOOL:
                    setattr(value, field.name, False)
                else:
                    setattr(value, field.name, 0)
                decoded = cls.FromString(value.SerializeToString())
                if not decoded.HasField(field.name) or decoded != value:
                    raise RuntimeError(f"optional zero fixture failed: {descriptor.full_name}.{field.name}")
                optionals += 1
    if count == 0:
        raise RuntimeError("no oneof fixtures generated")
    print(f"{count} binary oneof case and {optionals} optional-zero fixtures passed")


def generate(target: pathlib.Path) -> None:
    go, ts, cpp = (target / part for part in ("go", "ts", "cpp"))
    for directory in (go, ts, cpp):
        directory.mkdir(parents=True)
    run("protoc", f"-I{SCHEMA}", f"--plugin=protoc-gen-go={shutil.which('protoc-gen-go')}",
        f"--go_out={go}", "--go_opt=module=FlightStrips", "storage.proto", "wire.proto", cwd=SCHEMA)
    cluster_descriptor = target / "cluster.pb"
    run("protoc", f"-I{SCHEMA}", "--include_imports",
        f"--descriptor_set_out={cluster_descriptor}", "storage.proto", "wire.proto", cwd=SCHEMA)
    cluster_files = descriptor_pb2.FileDescriptorSet()
    cluster_files.ParseFromString(cluster_descriptor.read_bytes())
    request = plugin_pb2.CodeGeneratorRequest()
    request.file_to_generate.extend(("storage.proto", "wire.proto"))
    request.parameter = "target=ts"
    request.proto_file.extend(cluster_files.file)
    plugin = ROOT / "frontend/node_modules/@bufbuild/protoc-gen-es/bin/protoc-gen-es"
    response_bytes = subprocess.check_output(["node", str(plugin)], input=request.SerializeToString())
    response = plugin_pb2.CodeGeneratorResponse()
    response.ParseFromString(response_bytes)
    if response.error:
        raise RuntimeError(response.error)
    for file in response.file:
        output = ts / file.name
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(file.content.rstrip("\n") + "\n", encoding="utf-8")
    run("protoc", f"-I{SCHEMA}", f"--cpp_out={cpp}", "storage.proto", "wire.proto", cwd=SCHEMA)


def sync(source: pathlib.Path, destination: pathlib.Path, check: bool) -> None:
    expected = {p.name: p.read_bytes() for p in source.iterdir() if p.is_file()}
    suffixes = (".pb.go", "_pb.ts", ".pb.cc", ".pb.h")
    actual = {p.name: p.read_bytes() for p in destination.iterdir()
              if p.is_file() and p.name.endswith(suffixes)} if destination.exists() else {}
    if check:
        if expected != actual:
            raise RuntimeError(f"generated files differ in {destination}")
    else:
        destination.mkdir(parents=True, exist_ok=True)
        for name in actual.keys() - expected.keys():
            (destination / name).unlink()
        for name, data in expected.items():
            (destination / name).write_bytes(data)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true")
    parser.add_argument("--update-baseline", action="store_true",
                        help="reviewed contract change: replace the committed compatibility baseline")
    args = parser.parse_args()
    version("protoc", "26.1")
    version("protoc-gen-go", "v1.36.11")
    packages = json.loads((ROOT / "frontend/package.json").read_text(encoding="utf-8"))
    if (packages["dependencies"].get("@bufbuild/protobuf") != "2.14.0" or
            packages["devDependencies"].get("@bufbuild/protoc-gen-es") != "2.14.0"):
        raise RuntimeError("frontend Protobuf-ES packages must be pinned to 2.14.0")
    for name in ("storage.proto", "wire.proto"):
        if (SCHEMA / name).read_bytes() != (CANDIDATE / name).read_bytes():
            raise RuntimeError(f"{name} differs from normative candidate")
    with tempfile.TemporaryDirectory() as scratch:
        temp = pathlib.Path(scratch)
        files = descriptor(temp / "contract.pb")
        lint(files)
        roundtrip_oneofs(files)
        if args.check or not args.update_baseline:
            breaking(files)
        else:
            BASELINE.write_bytes((temp / "contract.pb").read_bytes())
        generate(temp)
        for source, destination in ((temp / "go/pkg/events/cluster", GO_OUT),
                                    (temp / "ts", TS_OUT), (temp / "cpp", CPP_OUT)):
            sync(source, destination, args.check)
    print("cluster Protobuf contract verified" if args.check else "cluster Protobuf bindings generated")


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, subprocess.CalledProcessError) as error:
        sys.exit(str(error))
