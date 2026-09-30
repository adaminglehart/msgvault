package vector

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFuse_BothSignalsContribute(t *testing.T) {
	assert := assert.New(t)
	bm25 := []Hit{{MessageID: 1, Rank: 1}, {MessageID: 2, Rank: 2}, {MessageID: 3, Rank: 3}}
	vec := []Hit{{MessageID: 2, Rank: 1}, {MessageID: 4, Rank: 2}, {MessageID: 1, Rank: 3}}
	out := Fuse(bm25, vec, 60)
	require.Len(t, out, 4)
	// Msg 2: BM25 rank 2 (1/62) + vec rank 1 (1/61) ≈ 0.03251 → highest.
	// Msg 1: BM25 rank 1 (1/61) + vec rank 3 (1/63) ≈ 0.03226.
	assert.Equal(int64(2), out[0].MessageID, "top")
	// Msg 3 is only in BM25; msg 4 only in vec. Both should have one NaN.
	for _, h := range out {
		switch h.MessageID {
		case 3:
			assert.False(math.IsNaN(h.BM25Score), "msg 3 BM25 should be non-NaN (or zero; Score=0)")
			assert.True(math.IsNaN(h.VectorScore), "msg 3 VectorScore should be NaN, got %v", h.VectorScore)
		case 4:
			assert.True(math.IsNaN(h.BM25Score), "msg 4 BM25Score should be NaN, got %v", h.BM25Score)
		}
	}
}

func TestFuse_OnlyBM25(t *testing.T) {
	assert := assert.New(t)
	bm25 := []Hit{{MessageID: 1, Rank: 1}, {MessageID: 2, Rank: 2}}
	out := Fuse(bm25, nil, 60)
	require.Len(t, out, 2)
	assert.Equal(int64(1), out[0].MessageID)
	assert.Equal(int64(2), out[1].MessageID)
	for _, h := range out {
		assert.Truef(math.IsNaN(h.VectorScore),
			"msg %d VectorScore should be NaN for BM25-only, got %v", h.MessageID, h.VectorScore)
	}
}

func TestFuse_OnlyVector(t *testing.T) {
	vec := []Hit{{MessageID: 10, Rank: 1}, {MessageID: 20, Rank: 2}}
	out := Fuse(nil, vec, 60)
	require.Len(t, out, 2)
	assert.Equal(t, int64(10), out[0].MessageID, "top")
	for _, h := range out {
		assert.Truef(t, math.IsNaN(h.BM25Score),
			"msg %d BM25Score should be NaN for vector-only, got %v", h.MessageID, h.BM25Score)
	}
}

func TestFuse_Empty(t *testing.T) {
	out := Fuse(nil, nil, 60)
	assert.Empty(t, out)
}

// TestFuse_TiedRRFScoresStableByMessageID verifies that when two
// hits have identical RRF scores (e.g. swapped BM25/vector ranks
// that add to the same 1/(k+r1) + 1/(k+r2)), the returned order is
// deterministic — ascending by MessageID — rather than relying on
// Go map iteration, which would scramble ties across invocations.
func TestFuse_TiedRRFScoresStableByMessageID(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)
	// Two hits with BM25 rank 1 / vec rank 2 vs. BM25 rank 2 / vec
	// rank 1 have identical RRF scores (1/61 + 1/62).
	bm25 := []Hit{{MessageID: 7, Rank: 1}, {MessageID: 3, Rank: 2}}
	vec := []Hit{{MessageID: 3, Rank: 1}, {MessageID: 7, Rank: 2}}
	for i := range 20 {
		out := Fuse(bm25, vec, 60)
		require.Lenf(out, 2, "iter %d", i)
		require.InDeltaf(out[0].RRFScore, out[1].RRFScore, 0, "iter %d: scores differ, not a tie scenario: %+v", i, out)
		assert.Equalf(int64(3), out[0].MessageID, "iter %d: want 3 first (ascending MessageID on tie)", i)
		assert.Equalf(int64(7), out[1].MessageID, "iter %d: want 7 second", i)
	}
}

func TestFuse_ScorePreservedFromInputs(t *testing.T) {
	bm25 := []Hit{{MessageID: 1, Rank: 1, Score: 5.5}}
	vec := []Hit{{MessageID: 1, Rank: 1, Score: 0.9}}
	out := Fuse(bm25, vec, 60)
	assert.InDelta(t, 5.5, out[0].BM25Score, 1e-6)
	assert.InDelta(t, 0.9, out[0].VectorScore, 1e-6)
}
