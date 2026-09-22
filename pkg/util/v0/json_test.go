package v0

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestUpdateNamespace(t *testing.T) {
	tests := []struct {
		name      string
		in        datatypes.JSON
		ns        string
		wantNS    string
		wantErr   bool
		errSubstr string
	}{
		{
			name:   "sets namespace for non-Gateway kinds",
			in:     datatypes.JSON(`{"kind":"Deployment","metadata":{"name":"x"}}`),
			ns:     "default",
			wantNS: "default",
		},
		{
			name:   "Gateway kind forces GatewaySystemNamespace",
			in:     datatypes.JSON(`{"kind":"Gateway","metadata":{"name":"x","namespace":"ignored"}}`),
			ns:     "default",
			wantNS: GatewaySystemNamespace,
		},
		{
			name:      "missing metadata returns error",
			in:        datatypes.JSON(`{"kind":"Deployment"}`),
			ns:        "default",
			wantErr:   true,
			errSubstr: `failed to find "metadata"`,
		},
		{
			name:      "invalid JSON returns error",
			in:        datatypes.JSON(`{"kind":`),
			ns:        "default",
			wantErr:   true,
			errSubstr: "failed to unmarshal JSON definition to map",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := UpdateNamespace(tt.in, tt.ns)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Fatalf("error=%q, want substring %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var got map[string]any
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatalf("failed to unmarshal output: %v; out=%s", err, string(out))
			}
			meta, ok := got["metadata"].(map[string]any)
			if !ok {
				t.Fatalf("output missing metadata: %v", got)
			}
			if gotNS, _ := meta["namespace"].(string); gotNS != tt.wantNS {
				t.Fatalf("namespace=%q, want %q; out=%v", gotNS, tt.wantNS, got)
			}
		})
	}
}

// TestJSONDefinitionsEqual covers the comparison callers use to decide whether
// a stored definition needs writing back.
func TestJSONDefinitionsEqual(t *testing.T) {
	definition := func(raw string) *datatypes.JSON {
		value := datatypes.JSON(raw)
		return &value
	}

	t.Run("identical bytes are equal", func(t *testing.T) {
		equal, err := JSONDefinitionsEqual(
			definition(`{"kind":"VirtualService","spec":{"hosts":["a"]}}`),
			definition(`{"kind":"VirtualService","spec":{"hosts":["a"]}}`),
		)
		require.NoError(t, err)
		assert.True(t, equal)
	})

	// this is the case the comparison exists for: a definition read back from
	// the database is not guaranteed to return byte-identical to what went in,
	// and comparing bytes would report a difference every time
	t.Run("reordered keys and whitespace are equal", func(t *testing.T) {
		equal, err := JSONDefinitionsEqual(
			definition(`{"kind":"VirtualService","spec":{"hosts":["a"]}}`),
			definition("{\n  \"spec\": {\"hosts\": [\"a\"]},\n  \"kind\": \"VirtualService\"\n}"),
		)
		require.NoError(t, err)
		assert.True(t, equal)
	})

	t.Run("a different value is not equal", func(t *testing.T) {
		equal, err := JSONDefinitionsEqual(
			definition(`{"kind":"VirtualService","spec":{"hosts":["a"]}}`),
			definition(`{"kind":"VirtualService","spec":{"hosts":["b"]}}`),
		)
		require.NoError(t, err)
		assert.False(t, equal)
	})

	// array order carries meaning in a Kubernetes manifest, so it is a
	// difference rather than noise
	t.Run("reordered array elements are not equal", func(t *testing.T) {
		equal, err := JSONDefinitionsEqual(
			definition(`{"hosts":["a","b"]}`),
			definition(`{"hosts":["b","a"]}`),
		)
		require.NoError(t, err)
		assert.False(t, equal)
	})

	t.Run("both absent are equal, one absent is not", func(t *testing.T) {
		equal, err := JSONDefinitionsEqual(nil, nil)
		require.NoError(t, err)
		assert.True(t, equal)

		equal, err = JSONDefinitionsEqual(definition(`{}`), nil)
		require.NoError(t, err)
		assert.False(t, equal)
	})

	t.Run("malformed JSON is an error, not a verdict", func(t *testing.T) {
		_, err := JSONDefinitionsEqual(definition(`{"kind":`), definition(`{}`))
		require.Error(t, err)
	})
}

// TestJSONDefinitionsEqualIgnoring covers setting aside fields a caller does not
// own, which is what lets it compare its own configuration against a stored copy
// another reconciler has written to.
func TestJSONDefinitionsEqualIgnoring(t *testing.T) {
	definition := func(raw string) *datatypes.JSON {
		value := datatypes.JSON(raw)
		return &value
	}
	namespace := []string{"metadata", "namespace"}

	t.Run("an ignored field present on one side only is not a difference", func(t *testing.T) {
		equal, err := JSONDefinitionsEqualIgnoring(
			definition(`{"kind":"Issuer","metadata":{"name":"a","namespace":"ns-1"}}`),
			definition(`{"kind":"Issuer","metadata":{"name":"a"}}`),
			namespace,
		)
		require.NoError(t, err)
		assert.True(t, equal)
	})

	t.Run("an ignored field differing on both sides is not a difference", func(t *testing.T) {
		equal, err := JSONDefinitionsEqualIgnoring(
			definition(`{"metadata":{"name":"a","namespace":"ns-1"}}`),
			definition(`{"metadata":{"name":"a","namespace":"ns-2"}}`),
			namespace,
		)
		require.NoError(t, err)
		assert.True(t, equal)
	})

	// setting a field aside must not hide anything else
	t.Run("a difference elsewhere still shows", func(t *testing.T) {
		equal, err := JSONDefinitionsEqualIgnoring(
			definition(`{"metadata":{"name":"a","namespace":"ns-1"}}`),
			definition(`{"metadata":{"name":"b"}}`),
			namespace,
		)
		require.NoError(t, err)
		assert.False(t, equal)
	})

	t.Run("an ignored path that is not there is not an error", func(t *testing.T) {
		equal, err := JSONDefinitionsEqualIgnoring(
			definition(`{"kind":"Issuer"}`),
			definition(`{"kind":"Issuer"}`),
			namespace,
			[]string{"status", "conditions"},
		)
		require.NoError(t, err)
		assert.True(t, equal)
	})

	t.Run("with nothing ignored it is a plain comparison", func(t *testing.T) {
		equal, err := JSONDefinitionsEqualIgnoring(
			definition(`{"metadata":{"namespace":"ns-1"}}`),
			definition(`{"metadata":{}}`),
		)
		require.NoError(t, err)
		assert.False(t, equal)
	})
}
