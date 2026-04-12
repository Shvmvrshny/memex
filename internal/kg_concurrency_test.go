package memex

import (
	"fmt"
	"sync"
	"testing"
)

func TestKG_WAL_ConcurrentReadersWriter(t *testing.T) {
	kg := newTestKG(t)

	// Seed an initial fact
	_, err := kg.RecordFact("service", "uses", "sqlite", "", "test", true)
	if err != nil {
		t.Fatalf("seed fact: %v", err)
	}

	const numReaders = 20
	const numWrites = 10

	var wg sync.WaitGroup
	readerErrors := make([]error, numReaders)

	// 20 concurrent readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < numWrites; j++ {
				facts, err := kg.QueryEntity("service", "")
				if err != nil {
					readerErrors[idx] = fmt.Errorf("reader %d iter %d: %w", idx, j, err)
					return
				}
				_ = facts
			}
		}(i)
	}

	// 1 writer replacing the singular fact
	wg.Add(1)
	go func() {
		defer wg.Done()
		objects := []string{"qdrant", "postgres", "sqlite", "qdrant", "redis", "qdrant", "sqlite", "qdrant", "postgres", "qdrant"}
		for _, obj := range objects {
			_, err := kg.RecordFact("service", "uses", obj, "", "test", true)
			if err != nil {
				t.Errorf("writer error: %v", err)
				return
			}
		}
	}()

	wg.Wait()

	// No reader errors
	for i, err := range readerErrors {
		if err != nil {
			t.Errorf("reader %d failed: %v", i, err)
		}
	}

	// Exactly one active fact for (service, uses)
	facts, err := kg.QueryEntity("service", "")
	if err != nil {
		t.Fatalf("final query: %v", err)
	}
	active := 0
	for _, f := range facts {
		if f.Subject == "service" && f.Predicate == "uses" {
			active++
		}
	}
	if active != 1 {
		t.Errorf("expected exactly 1 active (service, uses) fact, got %d: %+v", active, facts)
	}
}

func TestKG_WAL_ExpireNeverDeletes(t *testing.T) {
	kg := newTestKG(t)

	id, _ := kg.RecordFact("svc", "depends_on", "cache", "", "test", false)
	kg.ExpireFact(id, "")

	// Expired fact must still exist in history
	history, err := kg.History("svc")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	found := false
	for _, f := range history {
		if f.ID == id && f.ValidUntil != "" {
			found = true
		}
	}
	if !found {
		t.Error("expired fact must be preserved in History with valid_until set, not deleted")
	}

	// Must not appear in current query
	current, _ := kg.QueryEntity("svc", "")
	for _, f := range current {
		if f.ID == id {
			t.Error("expired fact must not appear in current QueryEntity")
		}
	}
}
