# M3-19n Web: add/edit library dialog, folder browser, permission help
Owner: mivo-frontend
Depends on: M3-19m
Files/areas: `web/apps/web/src/features/settings/**` (dialog, folder browser dialog, kind
tiles, permission help, hooks, locales, tests); the M3-19m page only to open the dialog

## Goal
An admin adds or edits a library in one dialog: kind as tiles, name, one or more folders picked
in a server-side folder browser, metadata language and the advanced options milestone 3
supports. When the server cannot read a folder, the dialog explains how to grant access for the
installation at hand. UX: LS-01, LS-02, LS-03 (M3 subset).

## Acceptance criteria
- [ ] Dialog (Radix, focus trapped, Esc closes, unsaved-changes guard): tab or section
      **General** – kind tiles for `systemInfo.libraryKinds` only (Movies, Shows in M3; kind
      read-only when editing, with the hint "create a new library to change the kind"), name
      (prefilled from the last element of the first chosen folder until the user edits it),
      metadata language (select: server default, English, Deutsch and the other languages of the
      list in the Notes; hint "used for online metadata from a later version"); **Folders** – list
      with remove buttons and **Add folder**; **Advanced** – behind an "Expert settings" switch
      (UX principle 7): Find extras, Read NFO files, Use local artwork (default on). One
      **Save** button.
- [ ] Folder browser dialog: root picker from `listFolderRoots` (unavailable roots disabled with a
      marker), breadcrumb of the current path, "Up" button, folder list from `listFolders`
      (keyboard navigable list; Enter opens, a **Select this folder** button chooses the current
      folder), entries with `selectable=false` show their reason (`reserved`, `in_use`) and
      cannot be chosen; `truncated` shows "Only the first 1000 folders are shown".
- [ ] **Permission help**: shown when `unreadableCount > 0` ("N folders here cannot be read by
      the server") and for a `folder_unreadable` save error, chosen by `installKind`:
      `synology` – "In DSM open Control Panel › Shared Folder, select the shared folder, choose
      Edit › Permissions, pick 'System internal user' in the drop-down, tick **Read only** for
      `<serviceUser>`, Save, then reopen this folder list"; `docker` – "Mount the folder into
      the container below `/media` (for example `-v /volume1/video:/media/video:ro`) and
      restart the container"; `standard` – "Give the user `<serviceUser>` read access to the
      folder". en and de texts.
- [ ] Save calls `createLibrary` or `updateLibrary` (`{folderId}` for kept folders,
      `{rootId, path}` for new ones); each problem code shows a field-level message on the
      right folder or field (`folder_overlap` names the other library if known); success closes
      the dialog, invalidates queries and announces "Scanning <name>…" (live region).
- [ ] Tests (Vitest + Testing Library, fake API): add with two folders, name prefill and manual
      override, kind tiles from `libraryKinds`, edit keeps folder ids and hides kind change,
      every problem code mapped to its field, permission help per install kind and for
      `unreadableCount`, non-selectable entries, truncated notice, keyboard-only flow, axe
      check.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test` with coverage thresholds, `pnpm -C web build`.

## Notes
- Language list (BCP 47 values sent to the server): server default (empty), `en`, `de`, `fr`,
  `es`, `it`, `nl`, `pl`, `pt`, `pt-BR`, `sv`, `da`, `no`, `fi`, `cs`, `ja`, `ko`, `zh`. Names are
  shown with `Intl.DisplayNames` in the UI language (existing `languageName` helper).
- Folder state (current root, path, chosen folders) is local dialog state; the dialog's open
  state is a URL search param (UX principle 6) so reload keeps it open.
- Wording: "System internal user" and "Read only" match DSM's English UI; the German texts use
  DSM's German labels ("Systeminterner Benutzer", "Schreibgeschützt") – check against the
  owner's DSM in SPK-17's device test and correct there if needed.
