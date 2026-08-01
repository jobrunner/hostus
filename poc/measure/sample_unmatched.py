#!/usr/bin/env python3
"""Systematische Stichprobe (jede k-te Zeile, k = floor(N/n)) aus der
sortierten Liste nicht aufgeloester Trait-Taxa, mit Evidenz je Name:
existiert die Gattung in WCVP? gibt es dort einen Namen mit gleichem
Gattungs- + Epitheton-Praefix (5 Zeichen)? Das trennt "orthographische
Variante / anderes Konzept" von "Gattung in WCVP unbekannt"."""
import sys, os

root = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
names_path = os.path.join(root, "poc/measure/out/wcvp-names.txt")

names = set()
by_genus = {}
with open(names_path, encoding="utf-8") as f:
    for line in f:
        n = line.rstrip("\n")
        names.add(n)
        g = n.split(" ", 1)[0]
        by_genus.setdefault(g, []).append(n)

for list_path in sys.argv[1:]:
    with open(list_path, encoding="utf-8") as f:
        all_names = [l.rstrip("\n") for l in f if l.strip()]
    n = 20
    step = max(1, len(all_names) // n)
    sample = all_names[::step][:n]
    print(f"=== {os.path.basename(list_path)}: N={len(all_names)}, systematische Stichprobe jede {step}. Zeile, n={len(sample)} ===")
    for name in sample:
        parts = name.split()
        genus = parts[0]
        epi = parts[1] if len(parts) > 1 else ""
        cand = by_genus.get(genus, [])
        near = [c for c in cand if c.startswith(f"{genus} {epi[:5]}")][:3]
        print(f"{name}\tgenus_names_in_wcvp={len(cand)}\tnear={'; '.join(near) if near else 'NONE'}")
    print()
