package util_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ctoup.com/coreapp/pkg/shared/util"
)

// The zero value is the absent value. That is the whole contract, and it is the
// one a hand-written copy gets wrong: `pgtype.Text{String: s, Valid: true}`
// writes an empty string where the column wanted NULL, and the difference only
// shows up in a query that filters on IS NULL.

func TestTheZeroValueBecomesNull(t *testing.T) {
	assert.False(t, util.TextOrNull("").Valid, "empty string is NULL")
	assert.False(t, util.UUIDOrNull(uuid.Nil).Valid, "uuid.Nil is NULL")

	got := util.TextOrNull("hello")
	assert.True(t, got.Valid)
	assert.Equal(t, "hello", got.String)

	id := uuid.New()
	gotID := util.UUIDOrNull(id)
	assert.True(t, gotID.Valid)
	assert.Equal(t, [16]byte(id), gotID.Bytes)
}

// Whitespace is not absence for a uuid, and it is for text only via OrDefault —
// TextOrNull stores "   " because a caller who wrote spaces meant them.
func TestWhitespaceIsStoredNotDropped(t *testing.T) {
	assert.True(t, util.TextOrNull("   ").Valid)
	assert.Equal(t, "fallback", util.OrDefault("   ", "fallback"))
	assert.Equal(t, "real", util.OrDefault("real", "fallback"))
}

// The jsonb columns these feed are NOT NULL with defaults like "{}" — so the
// fallback is raw JSON text, not a Go value.
func TestJSONBFallsBackToRawJSON(t *testing.T) {
	b, err := util.JSONBOrDefault(nil, "{}")
	require.NoError(t, err)
	assert.JSONEq(t, "{}", string(b))

	b, err = util.JSONBOrDefault(map[string]int{"a": 1}, "{}")
	require.NoError(t, err)
	assert.JSONEq(t, `{"a":1}`, string(b))
}

func TestRemapJSONReachesTheStructTags(t *testing.T) {
	var out struct {
		Name string `json:"name"`
		N    int    `json:"n"`
	}
	require.NoError(t, util.RemapJSON(map[string]any{"name": "x", "n": 3}, &out))
	assert.Equal(t, "x", out.Name)
	assert.Equal(t, 3, out.N)
}

// The reason there are two truncations. Cutting a multi-byte string by BYTES
// splits a rune and renders as U+FFFD; cutting by runes does not. A caller
// picks by whether a person reads the result.
func TestTruncateByBytesVersusRunes(t *testing.T) {
	const s = "héllo wörld"

	assert.Equal(t, s, util.Truncate(s, 100), "under the limit is untouched")
	assert.Equal(t, s, util.TruncateRunes(s, 100))

	byRunes := util.TruncateRunes(s, 4)
	assert.Equal(t, "héll…", byRunes)
	assert.NotContains(t, byRunes, "�", "cutting by runes never splits one")

	assert.Equal(t, len([]rune("héll"))+1, len([]rune(byRunes)))
}
