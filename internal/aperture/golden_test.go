package aperture

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestGatewayConfigRoundTripsWithoutSemanticChange(t *testing.T) {
	raw, err := os.ReadFile("testdata/gateway.hujson")
	if err != nil {
		t.Fatal(err)
	}

	got := rendered(t, applyUnchanged(t, string(raw)))

	if want := tree(t, string(raw)); !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Errorf("round trip changed the document\ngot:\n%s\nwant:\n%s", gotJSON, wantJSON)
	}
}
