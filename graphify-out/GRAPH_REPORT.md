# Graph Report - skylex  (2026-06-15)

## Corpus Check
- 76 files · ~49,146 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 7 nodes · 4 edges · 3 communities (1 shown, 2 thin omitted)
- Extraction: 100% EXTRACTED · 0% INFERRED · 0% AMBIGUOUS
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `2cdc1adc`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- [[_COMMUNITY_Community 0|Community 0]]
- [[_COMMUNITY_Community 1|Community 1]]

## God Nodes (most connected - your core abstractions)
1. `$schema` - 1 edges
2. `plugin` - 1 edges
3. `graphify` - 1 edges

## Surprising Connections (you probably didn't know these)
- None detected - all connections are within the same source files.

## Import Cycles
- None detected.

## Communities (3 total, 2 thin omitted)

## Knowledge Gaps
- **3 isolated node(s):** `$schema`, `plugin`, `graphify`
  These have ≤1 connection - possible missing edges or undocumented components.
- **2 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **What connects `$schema`, `plugin`, `graphify` to the rest of the system?**
  _3 weakly-connected nodes found - possible documentation gaps or missing edges._