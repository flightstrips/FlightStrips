"""Verify the release-only binding with protoc 26.1; never touch cluster bindings."""
from pathlib import Path
import subprocess
import tempfile

root = Path(__file__).resolve().parents[2]
assert subprocess.check_output(['protoc', '--version'], text=True).strip() == 'libprotoc 26.1'
with tempfile.TemporaryDirectory() as temp:
    subprocess.run(['protoc', '-I', str(root / 'proto'), '--python_out=' + temp,
                    str(root / 'proto/release/v1/candidate.proto')], check=True)
    generated = Path(temp) / 'release/v1/candidate_pb2.py'
    checked = root / 'scripts/release/release/v1/candidate_pb2.py'
    assert generated.read_text(encoding='utf-8') == checked.read_text(encoding='utf-8'), 'regenerate release binding with protoc 26.1'
