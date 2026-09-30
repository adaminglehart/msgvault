package vector

import (
	"math"
	"sort"
)

// Fuse combines two ranked lists via Reciprocal Rank Fusion. rrfK is the
// standard RRF constant (60 is typical). Returns hits ordered by RRFScore DESC.
func Fuse(bm25, vec []Hit, rrfK int) []FusedHit {
	byID := make(map[int64]*FusedHit)
	for _, h := range bm25 {
		entry := getOrInit(byID, h.MessageID)
		entry.BM25Score = h.Score
		entry.RRFScore += 1.0 / float64(rrfK+h.Rank)
	}
	for _, h := range vec {
		entry := getOrInit(byID, h.MessageID)
		entry.VectorScore = h.Score
		entry.RRFScore += 1.0 / float64(rrfK+h.Rank)
	}

	out := make([]FusedHit, 0, len(byID))
	for _, h := range byID {
		out = append(out, *h)
	}
	// Sort by RRFScore descending with MessageID ascending as
	// tie-breaker. Tied RRF scores are realistic (e.g. two messages
	// with the same bm25/vector rank pair), and without a stable
	// secondary key the output order depends on map iteration,
	// which breaks pagination and reproducibility.
	sort.Slice(out, func(i, j int) bool {
		if out[i].RRFScore != out[j].RRFScore {
			return out[i].RRFScore > out[j].RRFScore
		}
		return out[i].MessageID < out[j].MessageID
	})
	return out
}

func getOrInit(m map[int64]*FusedHit, id int64) *FusedHit {
	if h, ok := m[id]; ok {
		return h
	}
	h := &FusedHit{MessageID: id, BM25Score: math.NaN(), VectorScore: math.NaN()}
	m[id] = h
	return h
}
