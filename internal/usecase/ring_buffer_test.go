package usecase_test

import (
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"

	"github.com/stretchr/testify/assert"
)

func TestRingBuffer_AppendAndSnapshot_NewestFirst(t *testing.T) {
	t.Parallel()
	rb := usecase.NewRingBuffer[int](3)
	rb.Append(1)
	rb.Append(2)
	rb.Append(3)
	assert.Equal(t, []int{3, 2, 1}, rb.Snapshot())
}

func TestRingBuffer_DropsOldestOverCapacity(t *testing.T) {
	t.Parallel()
	rb := usecase.NewRingBuffer[int](3)
	for i := 1; i <= 5; i++ {
		rb.Append(i)
	}
	// capacity=3 なので最新 3 件 (5,4,3) のみ
	assert.Equal(t, []int{5, 4, 3}, rb.Snapshot())
}

func TestRingBuffer_EmptyReturnsEmptySlice(t *testing.T) {
	t.Parallel()
	rb := usecase.NewRingBuffer[string](10)
	assert.Equal(t, []string{}, rb.Snapshot())
}

func TestRingBuffer_ConcurrentAppendIsSafe(t *testing.T) {
	t.Parallel()
	rb := usecase.NewRingBuffer[int](100)
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(start int) {
			for j := 0; j < 10; j++ {
				rb.Append(start*10 + j)
			}
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}
	snap := rb.Snapshot()
	assert.Len(t, snap, 100)
}
