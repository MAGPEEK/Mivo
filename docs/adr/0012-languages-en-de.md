# 0012 English base, complete German, more languages through Weblate

- Status: Accepted
- Date: 2026-10-29
- Source: decision table in `docs/plan.md`

## Context

The owner is German-speaking. The project targets an international open-source audience.

## Decision

- English is the language for code, comments, docs, commits and the UI base strings.
- German (`de`) is **complete** for every release. A missing German key is a test failure.
- Web i18n uses i18next with JSON resources per language. Detection uses the browser
  language and falls back to English. Users can change the language in the settings later.
- The server returns machine-readable error codes (problem details, ADR 0017). Clients
  translate them. Server log messages stay English.
- More languages are added later through Weblate. The metadata language is a separate
  setting.

## Consequences

- Every UI task adds `en` and `de` strings in the same change.
- A Vitest test checks that `de` has exactly the keys of `en` and no empty values.
