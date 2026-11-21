"""Regenerate the pinned Techbuddie BIN master snapshot (CC BY 4.0)."""
import argparse
import csv
import gzip
import hashlib
import io
import os
from pathlib import Path
import re
import tempfile
import urllib.request

REVISION = "fcc907c81cf02b12e396b0c12a8c16472b47619a"
SOURCE = "Techbuddie-Solutions/binlist-data"
URL = f"https://raw.githubusercontent.com/{SOURCE}/{REVISION}/bins.csv"
SHA256 = "50e9d57ae6c7b04c17a18c74aa3c9b0b07962ec4ba2fbe8d1ff3b6db00bd7615"
OUTPUT = Path(__file__).resolve().parents[1] / "data" / "bins.csv.gz"
MAX_DOWNLOAD = 50 * 1024 * 1024
HEADER = ["prefix", "scheme", "funding", "issuer", "countryCode", "recordSource", "seen"]


def clean(value):
    value = " ".join(value.split())
    return "" if value.lower() in {"-", "n/a", "none", "null", "unknown"} else value


def build(content):
    if hashlib.sha256(content).hexdigest() != SHA256:
        raise ValueError("BIN source does not match the pinned SHA-256")
    reader = csv.DictReader(io.StringIO(content.decode("utf-8-sig"), newline=""))
    records = {}
    legacy = 0
    for row in reader:
        prefix = row["bin"].strip()
        if re.fullmatch(r"[0-9]{5}", prefix):
            legacy += 1
            continue
        if not re.fullmatch(r"[0-9]{6,8}", prefix) or prefix in records:
            raise ValueError(f"invalid or duplicate BIN prefix {prefix!r}")
        scheme = clean(row["brand"]).lower()
        funding = clean(row["type"]).lower()
        if "prepaid" in clean(row["category"]).lower().split():
            funding = "prepaid"
        country = clean(row["alpha_2"]).upper()
        if not re.fullmatch(r"[A-Z]{2}", country):
            country = ""
        records[prefix] = (scheme, funding, clean(row["issuer"]), country, clean(row["source"]), clean(row["seen"]))

    with tempfile.NamedTemporaryFile(dir=OUTPUT.parent, delete=False) as raw:
        temporary = Path(raw.name)
        try:
            with gzip.GzipFile(filename="", fileobj=raw, mode="wb", mtime=0) as compressed:
                with io.TextIOWrapper(compressed, encoding="utf-8", newline="") as stream:
                    writer = csv.writer(stream, lineterminator="\n")
                    writer.writerow([SOURCE, REVISION, len(records)])
                    writer.writerow(HEADER)
                    for prefix in sorted(records):
                        writer.writerow((prefix, *records[prefix]))
            raw.flush()
            temporary.chmod(0o644)
            os.replace(temporary, OUTPUT)
        finally:
            temporary.unlink(missing_ok=True)
    print(f"Packed {len(records):,} BIN prefixes ({legacy} legacy five-digit entries omitted) into {OUTPUT}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source-file", type=Path, help="Use a local copy of the pinned upstream CSV")
    args = parser.parse_args()
    if args.source_file:
        with args.source_file.open("rb") as source:
            content = source.read(MAX_DOWNLOAD + 1)
    else:
        with urllib.request.urlopen(URL, timeout=60) as source:
            content = source.read(MAX_DOWNLOAD + 1)
    if len(content) > MAX_DOWNLOAD:
        raise ValueError("BIN source exceeds the download size limit")
    build(content)
