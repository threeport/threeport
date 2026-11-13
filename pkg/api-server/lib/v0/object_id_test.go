package v0

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseObjectID(t *testing.T) {
	for _, test := range []struct {
		raw     string
		want    uint
		wantErr bool
	}{
		{raw: "1", want: 1},
		{raw: "007", want: 7},
		{raw: "0", want: 0},
		{raw: strconv.FormatUint(uint64(^uint(0)), 10), want: ^uint(0)},
		{raw: "18446744073709551616", wantErr: true},
		{raw: "abc", wantErr: true},
		{raw: "1 OR 1=1", wantErr: true},
		{raw: "-1", wantErr: true},
		{raw: "+1", wantErr: true},
		{raw: " 1", wantErr: true},
		{raw: "1.0", wantErr: true},
		{raw: "", wantErr: true},
	} {
		t.Run(test.raw, func(t *testing.T) {
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
			c.SetParamNames(PathParamId)
			c.SetParamValues(test.raw)

			got, err := ParseObjectID(c)

			if test.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), strconv.Quote(test.raw))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}
