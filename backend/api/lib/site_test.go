package lib

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHomePageDescribesAfikaPOS(t *testing.T) {
	rec := httptest.NewRecorder()
	if !serveSite(rec, httptest.NewRequest(http.MethodGet, "/", nil)) {
		t.Fatal("home page was not handled")
	}
	body := rec.Body.String()
	for _, expected := range []string{"Every sale.", "independent South African shops", "stock", "profit", "Afika POS"} {
		if !strings.Contains(body, expected) {
			t.Errorf("home page does not contain %q", expected)
		}
	}
	for _, stale := range []string{"Investing, made more accessible", "portfolio overview", "US persons"} {
		if strings.Contains(body, stale) {
			t.Errorf("home page still contains unrelated product copy %q", stale)
		}
	}
}

func TestHomePageRejectsPost(t *testing.T) {
	rec := httptest.NewRecorder()
	if !serveSite(rec, httptest.NewRequest(http.MethodPost, "/", nil)) {
		t.Fatal("home page was not handled")
	}
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}
