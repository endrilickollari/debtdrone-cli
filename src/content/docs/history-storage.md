---
title: Local history storage
description: Understand DebtDrone's bounded, privacy-conscious local scan history format and recovery behavior.
---

DebtDrone defines a shared local store for reopening scan summaries without
retaining repository source. The store is the persistence foundation for the
headless CLI, MCP server, and interactive TUI.

:::note[Integration status]
The shared scan service records completed and partial headless CLI and TUI
scans. The MCP scan path is not connected yet. `debtdrone history` still
displays demonstration entries, and the TUI browser displays only its current
session until the history command and UI integration lands.
:::

## Storage location

History uses the operating system's user configuration directory:

| Platform | Default path |
|---|---|
| Linux and other Unix systems | `$XDG_CONFIG_HOME/debtdrone/history.json`, or `$HOME/.config/debtdrone/history.json` when `XDG_CONFIG_HOME` is unset |
| macOS | `$HOME/Library/Application Support/debtdrone/history.json` |
| Windows | `%AppData%\debtdrone\history.json` |

DebtDrone creates a missing application directory with owner-only permissions
and writes the history and lock files with owner read/write permissions where
the platform supports Unix permission bits. If the application directory
already exists, DebtDrone preserves its permissions instead of changing a
directory it may share with other local state.

## Version 1 format

The file contains a schema version and newest-first scan records:

```json
{
  "version": 1,
  "records": [
    {
      "id": "d53f9d84-66ef-49d5-bd28-93aaef1e3008",
      "repository": "debtdrone-cli",
      "started_at": "2026-09-01T11:58:00Z",
      "completed_at": "2026-09-01T12:00:00Z",
      "outcome": "completed",
      "summary": {
        "findings": 12,
        "critical": 1,
        "high": 2,
        "medium": 4,
        "low": 5,
        "technical_debt_hours": 3.5,
        "warnings": 0,
        "analyzer_failures": 0
      }
    }
  ]
}
```

Records use generated UUIDs and UTC timestamps. Outcomes are `completed` or
`partial`. Aggregate counts are validated before they are written.

## Privacy boundary

The persisted format is an allowlist. It stores only:

- the record ID and scan timestamps;
- a repository display name;
- the scan outcome;
- aggregate finding, severity, debt, warning, and analyzer-failure counts.

Repository display names contain only a bounded basename. DebtDrone removes
parent directories, URL credentials, query strings, fragments, and control
characters. This prevents a local history entry from exposing a username,
absolute workspace location, or credential embedded in a repository URL.

The store has no fields for source contents, snippets, finding messages,
credentials, raw environment variables, absolute repository paths, SaaS user
IDs, organization IDs, or billing data.

## Retention and size bounds

Default history limits are:

| Bound | Default |
|---|---|
| Retention | 90 days |
| Maximum records | 200 |
| Maximum encoded file size | 2 MiB |

Expired records are removed when history is recorded or listed. Records are
ordered by completion time, and the oldest entries are removed first when the
count or encoded-size limit is reached. These limits prevent history from
growing without bound even when scans run frequently.

## Atomicity and concurrency

Every update follows one persistence path:

1. acquire the history file lock;
2. read and validate the current file;
3. apply retention and size limits;
4. write an owner-only temporary file in the same directory;
5. flush it and atomically replace the prior file;
6. flush the parent directory where the operating system supports it.

The lock coordinates independent DebtDrone processes as well as concurrent
operations in one process. Cancellation stops a process waiting for the lock.
If encoding, flushing, or replacement fails, the previous history file remains
in place.

## Corruption and incompatible versions

DebtDrone validates the entire file before changing it. It rejects malformed
JSON, missing or duplicate IDs, invalid counts or timestamps, duplicate JSON
documents, and unknown version 1 fields.

A newer schema fails with an instruction to upgrade DebtDrone before strict
version 1 decoding occurs. Older schemas request migration or moving the file
aside. Corrupt or incompatible data is never silently overwritten.

To recover manually, move `history.json` to a backup location and run DebtDrone
again. Keep the backup if it contains records you may want to inspect or
migrate later.
