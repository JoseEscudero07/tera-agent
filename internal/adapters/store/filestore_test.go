package store

import (
	"path/filepath"
	"testing"
)

func TestStore_DedupePersistsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.json")

	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Seen("job-1") {
		t.Fatal("unexpected seen before MarkDone")
	}
	s.MarkDone("job-1")
	if !s.Seen("job-1") {
		t.Fatal("MarkDone did not register")
	}

	// Simulate a restart: a fresh Store loading the same file.
	s2, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if !s2.Seen("job-1") {
		t.Fatal("dedupe not persisted across restart (would reprint the job)")
	}
	if s2.Seen("job-2") {
		t.Fatal("unknown job reported as seen")
	}
}

func TestStore_PendingBufferPersistsAndClears(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.json")

	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	s.AddPending([]byte(`{"type":"job_completed","id":"a"}`))
	s.AddPending([]byte(`{"type":"job_failed","id":"b"}`))

	// Reload and drain.
	s2, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	got := s2.TakePending()
	if len(got) != 2 {
		t.Fatalf("pending = %d, want 2 (persisted across restart)", len(got))
	}
	if len(s2.TakePending()) != 0 {
		t.Fatal("pending not cleared after TakePending")
	}
}
