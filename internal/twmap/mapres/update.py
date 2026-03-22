#!/usr/bin/env python3
"""Download all DDNet default external tilesets and the upstream license from the ddnet repo."""
import json
import os
import sys
import urllib.request

GITHUB_API = "https://api.github.com/repos/ddnet/ddnet/contents/data/mapres"
RAW_BASE = "https://raw.githubusercontent.com/ddnet/ddnet/master/data/mapres"
LICENSE_URL = "https://raw.githubusercontent.com/ddnet/ddnet/master/license.txt"

def main():
    dest = sys.argv[1] if len(sys.argv) > 1 else "internal/twmap/mapres"
    os.makedirs(dest, exist_ok=True)

    # ── Fetch upstream license ────────────────────────────────────────────
    license_path = os.path.join(dest, "LICENSE")
    urllib.request.urlretrieve(LICENSE_URL, license_path)
    print(f"Downloaded upstream license → {license_path}")

    # ── Fetch tileset PNGs ────────────────────────────────────────────────
    req = urllib.request.Request(GITHUB_API, headers={"User-Agent": "teeworlds-asset-service/1.0"})
    data = json.loads(urllib.request.urlopen(req).read())
    pngs = sorted(f["name"] for f in data if f["name"].endswith(".png"))

    print(f"Downloading {len(pngs)} tilesets to {dest}/")
    for name in pngs:
        url = f"{RAW_BASE}/{name}"
        out = os.path.join(dest, name)
        urllib.request.urlretrieve(url, out)
        sz = os.path.getsize(out)
        print(f"  {name} ({sz:,} bytes)")

    print(f"Done: {len(pngs)} files + LICENSE")

if __name__ == "__main__":
    main()
