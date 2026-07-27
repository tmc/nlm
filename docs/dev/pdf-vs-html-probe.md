# NotebookLM PDF versus HTML probe

## Setup

Document: *Attention Is All You Need* (arXiv:1706.03762).

- HTML: <https://ar5iv.labs.arxiv.org/html/1706.03762>
- PDF URL: <https://arxiv.org/pdf/1706.03762>
- Downloaded PDF: 2,215,244 bytes; SHA-256 `bdfaa68d8984f0dc02beaca527b76f207d99b666d31d1da728ee0728182df697`

Both URL forms returned successful HTTP responses before ingestion. A fresh
throwaway notebook named `probe: pdf-vs-html 1706.03762` remains available for
inspection; its identifier is omitted here.

Notebook creation initially failed even though `nlm account` reported 494/500.
The authoritative `nlm notebook list` had 494 rows, but the service reported a
limit. I removed the clearly disposable, two-source April probe
`codex-stale-row-probe` and retried.

## Indexed-source comparison

| Form | `source read` bytes | Body evidence | Fidelity observation |
| --- | ---: | --- | --- |
| HTML URL | 45,682 | Introduction, Table 3, BLEU, and References present | Complete body, but HTML/math conversion has concatenated tokens and duplicated equation markup. |
| PDF URL | 40,055 | Introduction, Table 3, BLEU, and References present | Complete, readable body; spacing in the title/abstract is visibly cleaner than the HTML extraction. |
| Downloaded PDF file | 40,055 | Introduction, Table 3, BLEU, and References present | Initially failed because `--name PDF-file` replaced the `.pdf` upload filename. After the CLI fix preserves the basename for upload and renames after, this source indexed successfully and is byte-for-byte identical to PDF URL. |

HTML indexed 5,627 more bytes (14.0% more than PDF URL). Both usable URL
forms include the deep body/table and references; neither is front-matter-only.
The local-file failure was a CLI bug, not a NotebookLM PDF-extraction limit:
the resumable upload service selects the parser from the upload filename's
extension. Passing `--name PDF-file` had removed `.pdf`. The CLI now uploads
the original basename and applies the requested source title afterward.

Raw indexed reads and SHA-256s:

| Form | SHA-256 |
| --- | --- |
| HTML URL | `6270491c564d600dadd560e6cd5ed70bc128cd6e955889fd839ed595be33cf87` |
| PDF URL | `deb961d6d7258b4b06c741b10792394a50c8deec21eed2b0c33bf165e4cf7212` |
| Downloaded PDF | `deb961d6d7258b4b06c741b10792394a50c8deec21eed2b0c33bf165e4cf7212` |

## Reasoning probe

Prompt, separately scoped to each usable source: “In Table 3, what are the
development BLEU scores for the base configuration and row (A) with one
attention head? State both values and the difference in BLEU. Answer only from
this source.”

| Form | Result |
| --- | --- |
| HTML URL | Correct: base 25.8, one-head 24.9, difference 0.9 BLEU. Transcript SHA-256 `605bebea790604756ce0954fe88fadf02a2aa9855ad217afeacc5bae92ddde48`. |
| PDF URL | Correct: base 25.8, one-head 24.9, difference 0.9 BLEU. Transcript SHA-256 `529d23aa17a39421507e59de1079d03b5142d8c276395442c9c24207a307949f`. |
| Downloaded PDF file | No extra call: its server-indexed text is byte-for-byte identical to PDF URL, so it has the same available evidence. |

## Verdict

**VERDICT: there is no global winner for this rich paper. Prefer PDF URL or the
now-working PDF file for prose, citations, and mathematical notation; prefer
HTML for appendix figures and attention visualizations, where PDF extraction
includes fragmented vector-graphic text.** HTML retained 14.0% more indexed
bytes, but that is a coverage measure rather than a quality score. Both forms
answered the scoped deep table question correctly.
