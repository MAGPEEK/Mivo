# M4-04a Domain: default audio and subtitle track selection
Owner: mivo-backend
Depends on: none
Files/areas: `internal/domain/playback/tracks.go` (+ tests), `internal/domain/media` only if a flag is missing

## Goal
Without a manual choice the player gets the audio track and subtitle the user would expect: the
preferred language, not the commentary, forced subtitles when the audio is foreign (ADR 0030 §3, §4; DT-07, PL-11/12).

## Acceptance criteria
- [ ] `SelectAudio(description, preference) (streamIndex, bool)`: order = preferred language list
      (profile), `default` disposition, non-commentary/descriptive (title/disposition), most channels, first.
- [ ] `SelectSubtitle(description, audioLanguage, preference) (trackID, bool)`: off by default; a
      forced track in the audio language when audio differs from the first preferred language; honours an
      explicit "always on" preference with SDH/non-SDH choice.
- [ ] Language matching normalises ISO 639-1/-2B/-2T and `und`.
- [ ] `Request.AudioStreamIndex` and `SubtitleTrackID` override the policy and are validated against
      the file (unknown index = `ErrInvalid`).
- [ ] Tests: table-driven, no mocks, domain coverage ≥ 90 %, cases for 3-audio corpus layout.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
- Profile gets `PreferredAudioLanguages`, `PreferredSubtitleLanguages`, `SubtitleMode`
  (`off|forced|always`); bounded arrays like the rest of the profile (64 KiB body).
- Pure functions; track ids as defined in M4-02a.
