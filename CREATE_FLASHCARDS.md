# Flashcards Command

This WIP adds CLI support for creating NotebookLM flashcards from an existing
notebook.

## What It Does

Create flashcards from the sources already attached to a notebook:

```bash
nlm create-flashcards NOTEBOOK_ID
```

Generation is asynchronous. The command returns the new artifact ID, then you
can check progress with:

```bash
nlm artifacts NOTEBOOK_ID
```

## What Changed

- Added a custom R7cb6c encoder for flashcards creation.
- Added `api.Client.CreateFlashcards`.
- Added the `create-flashcards` CLI command.
- Added command validation coverage.
- Added artifact state handling so observed ready flashcards show as ready.

## Reverse-Engineered Wire Format

A live NotebookLM UI capture on May 3, 2026 showed that flashcards creation uses
the universal artifact creation RPC, `R7cb6c`, with:

- artifact type `4`
- descriptor flags `[1,4,2,3,6]`
- config slot 9: `[null,[1,null,null,null,null,null,[2,2]]]`

This is different from the initial guessed artifact type `6`. The guessed type
returned an API error, while the observed UI shape successfully created
flashcards through the CLI.

## Download Status

Flashcards can be created and listed from the CLI, but they currently do not
download like infographics or slide decks.

Observed artifact payloads expose static media URLs for:

- infographics: rendered `googleusercontent.com/notebooklm` image URLs
- slide decks: `contribution.usercontent.google.com/download` PDF/PPTX URLs

The observed flashcards artifact payload does not expose equivalent downloadable
media URLs. Because of that, `download-artifact` reports:

```text
artifact "ARTIFACT_ID" does not expose downloadable media URLs
```

That is intentional. The CLI should report the real API state instead of
creating a fake or empty file.

## Can Flashcard Download Be Reverse Engineered?

Probably, but it needs a different target than the infographic and slide deck
downloads.

Infographic and slide deck download worked because NotebookLM exposed static
media URLs in artifact metadata. Flashcards appear to be rendered in the UI from
structured artifact data or a follow-up RPC rather than from a direct file URL.
To add a real download/export command, the next reverse-engineering pass should
capture the network activity when:

- opening the generated flashcards artifact
- flipping or paging through cards
- using any UI share, copy, print, export, or overflow actions

Useful outcomes would be:

- an RPC that returns the card front/back content
- a hidden export endpoint
- a share/render endpoint that includes card data
- enough structured payload to export flashcards as Markdown, JSON, or CSV

Until one of those is found, the implemented behavior is:

- `nlm create-flashcards NOTEBOOK_ID` creates flashcards
- `nlm artifacts NOTEBOOK_ID` shows readiness
- `nlm download-artifact NOTEBOOK_ID FLASHCARDS_ID` fails honestly when no
  downloadable media URL exists

## Usage

Create flashcards:

```bash
nlm create-flashcards NOTEBOOK_ID
```

Check status:

```bash
nlm artifacts NOTEBOOK_ID
```

Attempt download:

```bash
nlm download-artifact NOTEBOOK_ID FLASHCARDS_ID ./flashcards-output
```

If NotebookLM still does not expose flashcard media URLs, the command returns a
clear error instead of writing an invalid file.

## Verification

Focused tests:

```bash
go test -count=1 ./gen/method ./cmd/nlm
```

Live testing was performed against a private NotebookLM notebook. The exact
notebook, source, and artifact IDs are intentionally not committed because they
are account-linked resource identifiers.
