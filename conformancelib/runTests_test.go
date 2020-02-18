package conformance

import "testing"

func TestLoad(t *testing.T) {
	if err := runTests("./creds.json"); err != nil {
		t.Error(err)
	}
}
