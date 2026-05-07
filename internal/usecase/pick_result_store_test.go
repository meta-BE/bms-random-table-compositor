package usecase_test

import (
	"sync"
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	"github.com/stretchr/testify/require"
)

func TestPickResultStore_SetGetDelete(t *testing.T) {
	s := usecase.NewPickResultStore()

	_, ok := s.Get("missing")
	require.False(t, ok)

	r := domain.PickResult{
		PublishedTableID: "PUB1",
		GeneratedAt:      time.Unix(1700000000, 0),
		SeedKey:          "20260507",
	}
	s.Set("PUB1", r)

	got, ok := s.Get("PUB1")
	require.True(t, ok)
	require.Equal(t, "PUB1", got.PublishedTableID)
	require.Equal(t, "20260507", got.SeedKey)

	s.Delete("PUB1")
	_, ok = s.Get("PUB1")
	require.False(t, ok)
}

func TestPickResultStore_Snapshot_ReturnsCopy(t *testing.T) {
	s := usecase.NewPickResultStore()
	s.Set("A", domain.PickResult{PublishedTableID: "A"})
	s.Set("B", domain.PickResult{PublishedTableID: "B"})

	snap := s.Snapshot()
	require.Len(t, snap, 2)

	// snapshot 改変が store に影響しないこと
	delete(snap, "A")
	_, ok := s.Get("A")
	require.True(t, ok)
}

func TestPickResultStore_ConcurrentAccess(t *testing.T) {
	s := usecase.NewPickResultStore()
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := "P"
			s.Set(id, domain.PickResult{PublishedTableID: id})
			s.Get(id)
			if i%4 == 0 {
				s.Delete(id)
			}
		}(i)
	}
	wg.Wait()
}
