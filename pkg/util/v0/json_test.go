package v0

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// TestUpdateNamespace covers the namespace written onto a stored definition.
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
			name:   "keeps an ingress operator namespace",
			in:     datatypes.JSON(`{"kind":"Deployment","metadata":{"name":"nginx-tcp","namespace":"nukleros-ingress-system"}}`),
			ns:     "workload-1",
			wantNS: "nukleros-ingress-system",
		},
		{
			name:   "keeps a cert-manager operator namespace",
			in:     datatypes.JSON(`{"kind":"ServiceAccount","metadata":{"name":"cert-manager","namespace":"nukleros-certs-system"}}`),
			ns:     "workload-1",
			wantNS: "nukleros-certs-system",
		},
		{
			name:   "keeps a secrets operator namespace",
			in:     datatypes.JSON(`{"kind":"ServiceAccount","metadata":{"name":"external-secrets","namespace":"nukleros-secrets-system"}}`),
			ns:     "workload-1",
			wantNS: "nukleros-secrets-system",
		},
		{
			name:   "overwrites a user namespace",
			in:     datatypes.JSON(`{"kind":"Deployment","metadata":{"name":"app","namespace":"app"}}`),
			ns:     "workload-1",
			wantNS: "workload-1",
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
			// write the namespace and check the stored value
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

	// compare objects whose key order differs
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

	// compare lists whose element order differs
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

	t.Run("an int and the float from unmarshalling it compare equal", func(t *testing.T) {
		equal, err := JSONValuesEqual([]interface{}{int64(80)}, []interface{}{float64(80)})
		require.NoError(t, err)
		assert.True(t, equal)
	})

	t.Run("different values do not compare equal", func(t *testing.T) {
		equal, err := JSONValuesEqual([]interface{}{"http"}, []interface{}{"tcp"})
		require.NoError(t, err)
		assert.False(t, equal)
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

// TestCountsTowardManagedNamespace covers which objects count toward a namespace.
func TestCountsTowardManagedNamespace(t *testing.T) {
	cases := []struct {
		name string
		in   datatypes.JSON
		want bool
	}{
		{
			name: "user object counts",
			in:   datatypes.JSON(`{"kind":"Deployment","metadata":{"namespace":"app"}}`),
			want: true,
		},
		{
			name: "operator namespace does not count",
			in:   datatypes.JSON(`{"kind":"Deployment","metadata":{"namespace":"nukleros-ingress-system"}}`),
			want: false,
		},
		{
			name: "gateway does not count",
			in:   datatypes.JSON(`{"kind":"Gateway","metadata":{"namespace":""}}`),
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// check whether the object counts
			require.Equal(t, tc.want, CountsTowardManagedNamespace(tc.in))
		})
	}
}
