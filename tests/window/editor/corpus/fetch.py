#!/usr/bin/env python3
"""Download the recorded corpus into an explicit directory outside the repository.

Only test tooling uses Python. The input files are not redistributed with the
project. Review the original sources' usage conditions before other uses. Branch
URLs can change: a changed checksum fails instead of silently changing the test.
"""

import argparse
import hashlib
import json
from pathlib import Path
import tempfile
import urllib.request


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    args = parser.parse_args()
    directory = args.directory.resolve()
    repository = Path(__file__).resolve().parents[4]
    if directory == repository or repository in directory.parents:
        parser.error("choose a corpus directory outside the repository")
    directory.mkdir(parents=True, exist_ok=True)
    manifest = Path(__file__).with_name("testdata") / "corpus.json"
    for item in json.loads(manifest.read_text()):
        destination = directory / item["file"]
        if destination.exists():
            if hashlib.sha256(destination.read_bytes()).hexdigest() != item["sha256"]:
                raise ValueError(f"{destination}: existing file has a different checksum")
            print(f"verified {destination}", flush=True)
            continue
        temporary = None
        digest, size = hashlib.sha256(), 0
        try:
            request = urllib.request.Request(item["url"], headers={"User-Agent": "GOUI-SourceView-corpus-verification"})
            with urllib.request.urlopen(request, timeout=60) as response, tempfile.NamedTemporaryFile(
                dir=directory, prefix=destination.name + ".", suffix=".download", delete=False
            ) as out:
                temporary = Path(out.name)
                while chunk := response.read(64 * 1024):
                    size += len(chunk)
                    if size > item["bytes"]:
                        raise ValueError(f"{item['file']}: upstream size changed")
                    digest.update(chunk)
                    out.write(chunk)
            if size != item["bytes"] or digest.hexdigest() != item["sha256"]:
                raise ValueError(f"{item['file']}: upstream content changed; review and update the manifest explicitly")
            temporary.replace(destination)
            print(f"downloaded {destination}: {size} bytes", flush=True)
        finally:
            if temporary is not None:
                temporary.unlink(missing_ok=True)


if __name__ == "__main__":
    main()
