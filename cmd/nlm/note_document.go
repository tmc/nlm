package main

import (
	"sort"
	"strconv"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

// noteDocument is the format-neutral model of one rendered note. Rich is the
// document arm; Flat is the plain-arm Markdown fallback.
type noteDocument struct {
	Title     string
	Flat      string
	Rich      *richDocument
	Citations []api.Citation
}

func noteDocumentFromAPI(note *api.Note) noteDocument {
	if note == nil {
		return noteDocument{}
	}
	flat := note.GetRichText()
	if flat == "" {
		flat = note.GetContentText()
	}
	doc := noteDocument{
		Title: note.GetTitle(),
		Flat:  flat,
		Rich:  richDocumentFromProto(note.Rich),
	}
	doc.Citations = noteCitations(note)
	return doc
}

// noteCitations joins the note's own grounding details to the visible marker
// occurrences recovered from its flat Markdown projection.
func noteCitations(note *api.Note) []api.Citation {
	if note == nil || note.Rich == nil {
		return nil
	}
	records := noteGroundingRecords(note)
	if len(note.Rich.GetBody().GetAnnotations()) > 0 {
		return noteCitationsFromAnnotations(note, records)
	}
	visibleGroups := noteMarkerGroups(note)
	type occurrence struct {
		start, end int64
		record     noteGroundingRecord
	}
	var occurrences []occurrence
	for _, record := range records {
		for _, span := range record.detail.GetReplySpans() {
			occurrences = append(occurrences, occurrence{
				start:  span.GetStart(),
				end:    span.GetEnd(),
				record: record,
			})
		}
	}
	if len(occurrences) == 0 {
		return noteCitationsFromAnnotations(note, records)
	}
	sort.SliceStable(occurrences, func(i, j int) bool {
		if occurrences[i].start != occurrences[j].start {
			return occurrences[i].start < occurrences[j].start
		}
		return occurrences[i].end < occurrences[j].end
	})

	indexByRange := make(map[string][]int)
	nextByRange := make(map[string]int)
	next := 0
	out := make([]api.Citation, 0, len(occurrences))
	for _, occurrence := range occurrences {
		key := noteRangeKey(occurrence.start, occurrence.end)
		indices, ok := indexByRange[key]
		if !ok {
			if next < len(visibleGroups) {
				indices = visibleGroups[next]
			} else {
				indices = []int{next + 1}
			}
			indexByRange[key] = indices
			next++
		}
		position := nextByRange[key]
		index := indices[min(position, len(indices)-1)]
		nextByRange[key] = position + 1
		out = append(out, citationFromNoteGrounding(
			index,
			occurrence.record.source,
			occurrence.start,
			occurrence.end,
			occurrence.record.detail,
		))
	}
	return out
}

type noteMarkerOccurrence struct {
	Index int
	Start int
	End   int
}

func noteMarkerGroups(note *api.Note) [][]int {
	text := note.GetRichText()
	if text == "" {
		text = note.GetContentText()
	}
	matches := htmlMarkerRe.FindAllStringSubmatch(text, -1)
	out := make([][]int, 0, len(matches))
	for _, match := range matches {
		indices, ok := citationIndices(match[1])
		if ok {
			out = append(out, indices)
		}
	}
	return out
}

func noteRangeKey(start, end int64) string {
	return strconv.FormatInt(start, 10) + ":" + strconv.FormatInt(end, 10)
}

func noteCitationsFromAnnotations(note *api.Note, records []noteGroundingRecord) []api.Citation {
	annotations := append([]*pb.SourceAnnotation(nil), note.Rich.GetBody().GetAnnotations()...)
	sort.SliceStable(annotations, func(i, j int) bool {
		ri, rj := annotations[i].GetRange(), annotations[j].GetRange()
		if ri.GetStart() != rj.GetStart() {
			return ri.GetStart() < rj.GetStart()
		}
		return ri.GetEnd() < rj.GetEnd()
	})
	groups := noteMarkerGroups(note)
	var out []api.Citation
	group := -1
	position := 0
	var lastStart, lastEnd int64
	for _, annotation := range annotations {
		if annotation == nil || annotation.GetRange() == nil {
			continue
		}
		start, end := annotation.GetRange().GetStart(), annotation.GetRange().GetEnd()
		if group < 0 || start != lastStart || end != lastEnd {
			group++
			position = 0
			lastStart, lastEnd = start, end
		}
		indices := []int{group + 1}
		if group < len(groups) {
			indices = groups[group]
		}
		index := indices[min(position, len(indices)-1)]
		position++
		sourceID := annotation.GetSource().GetSourceId()
		detail := matchingNoteGrounding(records, sourceID, start, end)
		out = append(out, citationFromNoteGrounding(index, sourceID, start, end, detail))
	}
	return out
}

type noteGroundingRecord struct {
	source string
	detail *pb.Grounding
}

func noteGroundingRecords(note *api.Note) []noteGroundingRecord {
	keyed := note.Rich.GetGrounding()
	n := len(keyed)
	if len(note.Grounding) > n {
		n = len(note.Grounding)
	}
	out := make([]noteGroundingRecord, 0, n)
	for i := 0; i < n; i++ {
		var source string
		var detail *pb.Grounding
		if i < len(keyed) {
			source = keyed[i].GetSource().GetSourceId()
			detail = keyed[i].GetGrounding()
		}
		if i < len(note.Grounding) && note.Grounding[i] != nil {
			detail = note.Grounding[i]
		}
		out = append(out, noteGroundingRecord{source: source, detail: detail})
	}
	return out
}

func matchingNoteGrounding(records []noteGroundingRecord, source string, start, end int64) *pb.Grounding {
	var fallback *pb.Grounding
	for _, record := range records {
		if record.source != source {
			continue
		}
		if fallback == nil {
			fallback = record.detail
		}
		for _, span := range record.detail.GetReplySpans() {
			if span.GetStart() == start && span.GetEnd() == end {
				return record.detail
			}
		}
	}
	return fallback
}

func citationFromNoteGrounding(index int, source string, start, end int64, detail *pb.Grounding) api.Citation {
	citation := api.Citation{
		SourceIndex: index,
		SourceID:    source,
		StartChar:   int(start),
		EndChar:     int(end),
	}
	if detail == nil {
		return citation
	}
	if id := detail.GetSourceId().GetSourceId(); id != "" {
		citation.SourceID = id
	}
	citation.Confidence = detail.GetScore()
	citation.Excerpt, citation.ExcerptRuns = api.ExcerptFromGrounding(detail)
	if spans := detail.GetSourceSpans().GetSpans(); len(spans) > 0 {
		citation.SourceStart = int(spans[0].GetStart())
		citation.SourceEnd = int(spans[len(spans)-1].GetEnd())
	}
	for _, ref := range detail.GetChunkRefs() {
		if id := ref.GetChunk().GetSourceId(); id != "" {
			citation.ParentSourceID = id
			break
		}
	}
	return citation
}
