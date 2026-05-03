# Studio Artifact Commands

This WIP adds CLI support for NotebookLM Studio artifacts, with the main focus on creating and downloading infographics. It also adds artifact download support for slide decks and a verified `create-flashcards` command.

## What It Does

Create an infographic from the sources already attached to a notebook:

```bash
nlm create-infographic NOTEBOOK_ID "Create a visual summary of the key findings"
```

Create flashcards from the selected notebook sources:

```bash
nlm create-flashcards NOTEBOOK_ID
```

Generation is asynchronous. Creation commands return the new artifact ID, then you can check progress with:

```bash
nlm artifacts NOTEBOOK_ID
```

Once the artifact is ready, inspect or download generated media with:

```bash
nlm get-artifact ARTIFACT_ID
nlm download-artifact NOTEBOOK_ID ARTIFACT_ID [output]
```

`download-artifact` is the primary download command. It downloads whatever media NotebookLM exposes for the artifact. Infographics expose an image. Slide decks expose PDF/PPTX files. Flashcards currently do not expose downloadable media URLs in the observed NotebookLM payload.

Convenience aliases are also wired:

```bash
nlm download-infographic NOTEBOOK_ID INFOGRAPHIC_ID [filename]
nlm download-slides NOTEBOOK_ID SLIDE_DECK_ID [output-dir]
nlm download-flashcards NOTEBOOK_ID FLASHCARDS_ID [output-dir]
```

## What Changed

This feature follows the existing audio, video, and slide deck command patterns:

- Added a custom R7cb6c encoder for infographic artifact creation.
- Added a custom R7cb6c encoder for flashcards creation.
- Added `api.Client.CreateInfographic`.
- Added `api.Client.CreateFlashcards`.
- Added the `create-infographic` CLI command.
- Added the `create-flashcards` CLI command.
- Added CLI artifact media extraction/download support for infographics, slide decks, and similar Studio artifacts.
- Added the `create_infographic` MCP tool.
- Updated command docs and README examples.
- Added focused encoder and CLI validation tests.

The infographic wire shape uses artifact type `7`, the same source reference nesting as slide decks, and the existing universal artifact creation RPC.

Flashcards do not use the guessed artifact type `6`. A live UI capture showed that NotebookLM sends artifact type `4` with a flashcards-specific config payload.

## Reverse-Engineered Wire Format

A real NotebookLM UI capture on May 2, 2026 showed that infographic creation uses `R7cb6c` with:

- artifact type `7`
- descriptor flags `[1,4,2,3,6]`
- config shape `[[custom_instructions_or_null,null,null,1,2]]`

This differs from slide decks, which use descriptor flags `[1,4,2,3,6,5]` and a different config slot.

Live flashcards capture on May 3, 2026 showed `R7cb6c` with:

- artifact type `4`
- descriptor flags `[1,4,2,3,6]`
- config slot 9: `[null,[1,null,null,null,null,null,[2,2]]]`

Current NotebookLM also returns numeric artifact state `3` for Studio artifacts that the UI treats as ready. The CLI/MCP display maps that observed state to `ARTIFACT_STATE_READY` rather than the stale generated protobuf label.

## Artifact Download

`get-artifact` now uses the artifact list fallback when the direct artifact endpoint only returns a skeleton record. That exposes generated media URLs for artifacts such as infographics and slide decks:

```bash
nlm get-artifact ARTIFACT_ID
```

Download artifact media:

```bash
nlm download-artifact NOTEBOOK_ID ARTIFACT_ID [output]
```

Examples:

```bash
nlm download-artifact NOTEBOOK_ID INFOGRAPHIC_ID infographic.jpg
nlm download-artifact NOTEBOOK_ID SLIDE_DECK_ID ./slide-deck-output
```

For the live slide deck test, this wrote both:

- `01-The_Ensemble_Blueprint.pdf`
- `02-The_Ensemble_Blueprint.pptx`

Reverse-engineering note: slide decks expose `contribution.usercontent.google.com/download` links for PDF/PPTX, while infographics expose a rendered `googleusercontent.com/notebooklm` image. Those download URLs require the correct `authuser` query parameter and may require a browser-profile fallback, so the CLI can reuse the logged-in Chrome profile when a plain HTTP fetch receives `403`.

Flashcards are ready/listable through `artifacts`, but the observed artifact list payload does not expose downloadable media URLs for them. The CLI reports that honestly instead of fabricating an output file.

## Authentication

No Google API key is needed. This CLI uses NotebookLM browser session credentials:

- `NLM_COOKIES`
- `NLM_AUTH_TOKEN`
- Optional: `NLM_AUTHUSER` for multi-account profiles

The easiest setup is:

```bash
nlm auth
```

That extracts browser credentials and stores them for future runs.

## Usage

Create an infographic:

```bash
nlm create-infographic NOTEBOOK_ID "Create an executive summary infographic"
```

Check artifact status:

```bash
nlm artifacts NOTEBOOK_ID
```

Show the generated image URL:

```bash
nlm get-artifact ARTIFACT_ID
```

Download the generated image through the CLI:

```bash
nlm download-artifact NOTEBOOK_ID INFOGRAPHIC_ID infographic.jpg
```

Download the generated slide deck:

```bash
nlm download-artifact NOTEBOOK_ID SLIDE_DECK_ID slide-deck-output
```

Create flashcards:

```bash
nlm create-flashcards NOTEBOOK_ID
```

## MCP Tool

The MCP server now exposes:

```text
create_infographic
download_artifact
```

Input:

```json
{
  "notebook_id": "NOTEBOOK_ID",
  "instructions": "Create a visual summary of the key findings"
}
```

After the artifact is ready, MCP clients can download generated media with
`download_artifact` by passing `notebook_id`, `artifact_id`, and an optional
`output` path.

## Verification

Focused tests:

```bash
go test -count=1 ./gen/method ./cmd/nlm
go test -run '^$' ./internal/notebooklm/api
```

Earlier focused coverage also passed:

```bash
go test ./gen/method ./internal/auth ./internal/nlmmcp ./cmd/nlm
```

The full suite was not used as the release signal because pre-existing interactive-audio tests have unrelated local fixture/audio backend issues.

## Manual Test Plan

Build or install from this branch, then run the normal CLI commands:

```bash
go install ./cmd/nlm
```

Create an infographic:

```bash
nlm create-infographic NOTEBOOK_ID "Create an executive visual summary"
nlm artifacts NOTEBOOK_ID
```

Download the infographic:

```bash
nlm download-artifact NOTEBOOK_ID INFOGRAPHIC_ID ./infographic.jpg
```

Create flashcards:

```bash
nlm create-flashcards NOTEBOOK_ID
nlm artifacts NOTEBOOK_ID
```

Download a slide deck:

```bash
nlm download-artifact NOTEBOOK_ID SLIDE_DECK_ID ./slide-deck-output
```

Live testing was performed against a private NotebookLM notebook. The exact notebook,
source, and artifact IDs are intentionally not committed because they are account-linked
resource identifiers. Local verification covered:

- CLI-created infographic reaches ready state.
- CLI-downloaded infographic writes a JPEG image.
- CLI-downloaded slide deck writes PDF and PPTX files.
- CLI-created flashcards reach ready state.
- Flashcards do not expose downloadable media URLs in the observed artifact list payload.
