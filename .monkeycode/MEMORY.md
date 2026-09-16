# User Instruction Memory

This file records user instructions, preferences, and teachings for reference in future interactions.

## Format

### User Instruction Entry
User instruction entries should follow this format:

[User Instruction Summary]
- Date: [YYYY-MM-DD]
- Context: [Mentioned scenario or time]
- Instructions:
  - [Content of user teaching or instruction, described line by line]

### Project Knowledge Entry
Entries discovered by the Agent during task execution should follow this format:

[Project Knowledge Summary]
- Date: [YYYY-MM-DD]
- Context: Discovered by Agent while performing [specific task description]
- Category: [Operations & Deployment|Build Methods|Testing Methods|Troubleshooting & Debugging|Workflow & Collaboration|Environment Configuration]
- Instructions:
  - [Specific knowledge points, described line by line]

## Deduplication Strategy
- Before adding a new entry, check for similar or identical instructions.
- If a duplicate is found, skip the new entry or merge it with the existing one.
- When merging, update the context or date information.
- This helps avoid redundant entries and keeps the memory file tidy.

## Entries

[Project Knowledge Summary]
- Date: 2026-09-16
- Context: Discovered by Agent while building and testing the analyst suite
- Category: Build Methods
- Instructions:
  - Full suite is `go test -count=1 ./core/... .` from the repo root; 18 core packages plus the root package.
  - Build the CLI with `go build -o /tmp/axdata .`; smoke-test against a fresh data root with `--data-root /tmp/axdata_vN` so cache state never masks a bug.
  - The analysis commands expose `--format-json` as a bool flag. Writing anything to stdout alongside the JSON output breaks piping; status banners belong on stderr.
  - Flag names were wrong in early smoke tests: `market chart` uses `--bars` (not `--days`), `market watch` uses `--codes` (not `-c`), `portfolio analyze` uses `--weights CODE=PCT`.

[Project Knowledge Summary]
- Date: 2026-09-16
- Context: Discovered by Agent while debugging cache isolation between securities
- Category: Troubleshooting & Debugging
- Instructions:
  - `axdata query` hardcodes `LIMIT 10`, so `Total rows: N` is a display cap and never the table size. Count real rows with `cat <data-root>/data/core/<table>.parquet.count`.
  - Snapshot tables are shared across securities in one file, so a key-blind read-then-write cycle overwrites every other security's rows. Always pass a `where` filter through `cache.FetchTable`.
  - Realtime price sources disagree inside the sandbox: tencent `stock_zh_a_hist_tx` returns no data and sina `kline` returns 2048 rows regardless of `limit`, so chart data comes from the sina fallback path.
  - Bank stocks return no `business_scope` driver data, and old (pre-2013) 业绩预告 rows carry zero amount bands with no matching reported period; both cases are rendered as `-` with a reason string.

[Project Knowledge Summary]
- Date: 2026-09-16
- Context: Discovered by Agent while using background terminals for builds
- Category: Environment Configuration
- Instructions:
  - Background terminals run zsh, not sh: bare globs and `grep --include=*.go` fail with `no matches found`. Quote the glob or drop the pattern.
  - Background terminal log files show 0 bytes while the command is running; poll `background_terminal_output_path` then read the file, and end commands with `; echo DONE` instead of `${PIPESTATUS[0]}`.
