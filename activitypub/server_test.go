package activitypub

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetActor(t *testing.T) {
	s := NewServer(nil, nil)
	server := httptest.NewServer(s.Router)
	defer server.Close()
	resp, err := http.Get(fmt.Sprintf("%s/actor/bendean", server.URL))
	if err != nil {
		t.Errorf("getActor(): returned an unexpected err: got %v want %v", err, nil)
	}
	defer resp.Body.Close()
	gotStatus := resp.StatusCode
	wantStatus := http.StatusNotImplemented
	if gotStatus != wantStatus {
		t.Errorf("getActor(): returned an unexpected status: got %v want %v", gotStatus, wantStatus)
	}
}
