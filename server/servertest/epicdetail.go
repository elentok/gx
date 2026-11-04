package servertest

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/elentok/gx/server/wire"
)

// GetEpicDetail fetches /v1/epics/detail for address and returns the status
// code, the decoded body (only on 200) and the raw body.
func GetEpicDetail(t *testing.T, h *Harness, base, address string) (int, wire.EpicDetail, string) {
	t.Helper()
	code, body := WebDo(t, h, http.MethodGet, base+"/v1/epics/detail?address="+address)
	var d wire.EpicDetail
	if code == http.StatusOK {
		if err := json.Unmarshal([]byte(body), &d); err != nil {
			t.Fatalf("body %q: %v", body, err)
		}
	}
	return code, d, body
}

// RowOf returns the row of d with the given address, failing the test if absent.
func RowOf(t *testing.T, d wire.EpicDetail, address string) wire.EpicDetailRow {
	t.Helper()
	for _, r := range d.Rows {
		if r.Address == address {
			return r
		}
	}
	t.Fatalf("no row %s in %+v", address, d.Rows)
	return wire.EpicDetailRow{}
}
