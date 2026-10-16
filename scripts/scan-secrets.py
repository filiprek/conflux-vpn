#!/usr/bin/env python3
"""Scan tracked textual sources for private keys and common provider credentials."""
import re
import subprocess
import sys
from pathlib import Path
rules = [re.compile(rb'-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----'),
         re.compile(rb'\bAKIA[0-9A-Z]{16}\b'),
         re.compile(rb'\bgh[pousr]_[A-Za-z0-9]{30,}\b'),
         re.compile(rb'\bgithub_pat_[A-Za-z0-9_]{40,}\b')]
failed = False
for raw in subprocess.check_output(['git', 'ls-files', '-z']).split(b'\0'):
    if not raw:
        continue
    path = Path(raw.decode())
    if path.is_file() and path.stat().st_size <= 4_000_000:
        data = path.read_bytes()
        if any(rule.search(data) for rule in rules):
            print(f'Potential secret in {path}; inspect locally. Value suppressed.', file=sys.stderr)
            failed = True
sys.exit(1 if failed else 0)
