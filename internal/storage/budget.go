package storage

import (
	"fmt"
	"io"
	"sync/atomic"
)

type Budget struct{ remaining atomic.Int64 }

func NewBudget(bytes int64) *Budget            { b := &Budget{}; b.remaining.Store(bytes); return b }
func (b *Budget) Writer(w io.Writer) io.Writer { return &budgetWriter{budget: b, w: w} }

type budgetWriter struct {
	budget *Budget
	w      io.Writer
}

func (w *budgetWriter) Write(p []byte) (int, error) {
	for {
		left := w.budget.remaining.Load()
		if int64(len(p)) > left {
			return 0, fmt.Errorf("expanded archive exceeds byte limit")
		}
		if w.budget.remaining.CompareAndSwap(left, left-int64(len(p))) {
			break
		}
	}
	return w.w.Write(p)
}
