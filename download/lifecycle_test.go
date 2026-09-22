package download

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCloseCancelsActiveAndQueuedDownloads(t *testing.T) {
	started := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-r.Context().Done()
	}))
	defer server.Close()
	m, _ := newTestManager(t, Config{})
	m.Submit(context.Background(), commandFor(server, []byte("payload")))
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("download not started")
	}
	queued := commandFor(server, []byte("payload"))
	queued.JobID = "22222222-2222-4222-8222-222222222222"
	m.Submit(context.Background(), queued)
	done := make(chan struct{})
	go func() { m.Close(); m.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("workers did not stop")
	}
	m.Submit(context.Background(), queued) // Must not panic/send to a closed queue.
}
