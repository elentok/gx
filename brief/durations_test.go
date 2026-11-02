package brief

import (
	"maps"
	"testing"
)

func TestGoTestParse_RecordsElapsedPerTestInMilliseconds(t *testing.T) {
	out := `{"Action":"run","Package":"p","Test":"TestA"}
{"Action":"pass","Package":"p","Test":"TestA","Elapsed":1.004}
{"Action":"run","Package":"p","Test":"TestB"}
{"Action":"fail","Package":"p","Test":"TestB","Elapsed":0.25}
{"Action":"skip","Package":"p","Test":"TestC","Elapsed":0}
{"Action":"pass","Package":"p","Elapsed":2.5}
`
	got := GoTest{}.Parse([]byte(out), 1).TestDurations
	want := map[string]int64{"p.TestA": 1004, "p.TestB": 250}
	if !maps.Equal(got, want) {
		t.Errorf("TestDurations = %v, want %v", got, want)
	}
}

func TestVitestParse_RecordsDurationPerTestInMilliseconds(t *testing.T) {
	out := `{"numTotalTestSuites":1,"numTotalTests":3,"testResults":[{"name":"/a.test.ts","status":"passed","assertionResults":[` +
		`{"fullName":"a one","status":"passed","duration":12.6,"failureMessages":[]},` +
		`{"fullName":"a two","status":"failed","duration":3,"failureMessages":["x"]},` +
		`{"fullName":"a three","status":"pending","duration":null,"failureMessages":[]}]}]}`
	got := Vitest{}.Parse([]byte(out), 1).TestDurations
	want := map[string]int64{"/a.test.ts.a one": 13, "/a.test.ts.a two": 3}
	if !maps.Equal(got, want) {
		t.Errorf("TestDurations = %v, want %v", got, want)
	}
}
