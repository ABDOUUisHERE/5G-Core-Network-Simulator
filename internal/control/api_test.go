package control

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCrossOriginControlIsRejected(t *testing.T) {
	api := New(context.Background(), Clients{}, time.Second)
	r := httptest.NewRequest("POST", "http://127.0.0.1:8085/api/ues", strings.NewReader(`{}`))
	r.Header.Set("Origin", "https://untrusted.example")
	w := httptest.NewRecorder()
	api.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}

func TestJSONRejectsUnknownFieldsAndMultipleObjects(t *testing.T) {
	for _, input := range []string{`{"count":1,"typo":true}`, `{"count":1}{"count":2}`, strings.Repeat("x", 17000)} {
		r := httptest.NewRequest("POST", "/", strings.NewReader(input))
		w := httptest.NewRecorder()
		target := struct {
			Count int `json:"count"`
		}{}
		if err := decode(w, r, &target); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
}
func TestPercentileEmptyAndKnownData(t *testing.T) {
	if percentile(nil, .95) != 0 || percentile([]float64{10, 20, 30, 40, 50}, .5) != 30 {
		t.Fatal("incorrect percentile")
	}
}
