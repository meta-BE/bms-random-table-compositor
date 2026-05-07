package idgen_test

import (
	"sync"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/idgen"
	"github.com/stretchr/testify/require"
)

func TestULIDGenerator_New_NonEmpty(t *testing.T) {
	g := idgen.NewULID()
	id := g.New()
	require.NotEmpty(t, id)
	require.Len(t, id, 26, "ULID は 26 文字")
}

func TestULIDGenerator_New_Unique(t *testing.T) {
	g := idgen.NewULID()
	seen := map[string]struct{}{}
	for i := 0; i < 1000; i++ {
		id := g.New()
		_, dup := seen[id]
		require.Falsef(t, dup, "重複 ID 検出 i=%d id=%s", i, id)
		seen[id] = struct{}{}
	}
}

func TestULIDGenerator_New_ConcurrentSafe(t *testing.T) {
	g := idgen.NewULID()
	var wg sync.WaitGroup
	out := make(chan string, 200)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				out <- g.New()
			}
		}()
	}
	wg.Wait()
	close(out)
	seen := map[string]struct{}{}
	for id := range out {
		require.NotEmpty(t, id)
		_, dup := seen[id]
		require.Falsef(t, dup, "並行生成で重複: %s", id)
		seen[id] = struct{}{}
	}
}
