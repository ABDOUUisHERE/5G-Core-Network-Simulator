package store

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestAtomicRollbackAndIsolation(t *testing.T) {
	s := NewMemory("udm")
	ctx := context.Background()
	sentinel := errors.New("rollback")
	err := s.Run(ctx, func(tx *Tx) error {
		if err := tx.Put("subscribers", "one", map[string]int{"n": 1}); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	if err = s.Run(ctx, func(tx *Tx) error {
		var out map[string]int
		found, err := tx.Get("subscribers", "one", &out)
		if found {
			t.Fatal("uncommitted data visible")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err = s.Run(ctx, func(tx *Tx) error { return tx.Put("sessions", "escape", 1) }); err == nil {
		t.Fatal("cross-owner write allowed")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err = s.Run(canceled, func(*Tx) error { t.Fatal("canceled transaction ran"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestConcurrentTransactions(t *testing.T) {
	s := NewMemory("udm")
	var wg sync.WaitGroup
	for range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.Run(context.Background(), func(tx *Tx) error {
				var n int
				_, err := tx.Get("subscribers", "counter", &n)
				if err != nil {
					return err
				}
				return tx.Put("subscribers", "counter", n+1)
			}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if err := s.Run(context.Background(), func(tx *Tx) error {
		var n int
		_, err := tx.Get("subscribers", "counter", &n)
		if n != 40 {
			t.Fatalf("lost updates: %d", n)
		}
		all, err := All[int](tx, "subscribers")
		if err != nil || len(all) != 1 {
			t.Fatalf("list: %v %v", all, err)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
