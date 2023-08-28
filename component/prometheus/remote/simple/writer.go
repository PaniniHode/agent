package simple

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/agent/pkg/flow/logging"

	"github.com/grafana/agent/component/prometheus"
)

type writer struct {
	mut         sync.RWMutex
	parentId    string
	keys        []uint64
	currentKey  uint64
	to          *QueueManager
	store       *dbstore
	ctx         context.Context
	bookmarkKey string
	l           log.Logger
}

func newWriter(parent string, to *QueueManager, store *dbstore, l *logging.Logger) *writer {
	name := fmt.Sprintf("metrics_write_to_%s_parent_%s", to.Name(), parent)
	w := &writer{
		parentId:    parent,
		keys:        make([]uint64, 0),
		currentKey:  0,
		to:          to,
		store:       store,
		bookmarkKey: name,
		l:           log.With(l, "name", name),
	}
	v, found := w.store.GetBookmark(w.bookmarkKey)
	// If we dont have a bookmark then grab the oldest key.
	if !found {
		w.currentKey = w.store.GetOldestKey()
	} else {
		w.currentKey = v.Key
	}
	if w.currentKey == 0 {
		w.currentKey = 1
	}
	return w
}

func (w *writer) Start(ctx context.Context) {
	w.mut.Lock()
	w.ctx = ctx
	w.mut.Unlock()

	newKey := w.incrementKey()
	success := true

	var err error
	for {
		recoverableError := true
		timeOut := 1 * time.Second
		// If we got a new key or the previous record did not enqueue then continue trying to send.
		if newKey || !success {
			level.Info(w.l).Log("msg", "looking for signal", "key", w.currentKey)
			// Eventually this will expire from the TTL.
			val, signalFound := w.store.GetSignal(w.currentKey)
			if signalFound {
				switch v := val.(type) {
				case []prometheus.Sample:
					success, err = w.to.Append(ctx, v)
				case []prometheus.Metadata:
					success = w.to.AppendMetadata(v)
				case []prometheus.Exemplar:
					success, err = w.to.AppendExemplars(ctx, v)
				case []prometheus.FloatHistogram:
					success, err = w.to.AppendFloatHistograms(ctx, v)
				case []prometheus.Histogram:
					success, err = w.to.AppendHistograms(ctx, v)
				}
				if err != nil {
					// Let's check if it's an `out of order sample`. Yes this is some hand waving going on here.
					// TODO add metric for unrecoverable error
					if strings.Contains(err.Error(), "the sample has been rejected") {
						recoverableError = false
					}
					level.Error(w.l).Log("msg", "error sending samples", "err", err)
				}

				// We need to succeed or hit an unrecoverable error.
				if success || !recoverableError {
					// Write our bookmark of the last written record.
					err = w.store.WriteBookmark(w.bookmarkKey, &Bookmark{
						Key: w.currentKey,
					})
					if err != nil {
						level.Error(w.l).Log("msg", "error writing bookmark", "err", err)
					}
				}
			}
		}

		if success || !recoverableError {
			newKey = w.incrementKey()
		}

		// If we were successful and have a newkey the quickly move on.
		// If the queue is not full then give time for it to send.
		if success && newKey {
			timeOut = 10 * time.Millisecond
		}

		tmr := time.NewTimer(timeOut)
		select {
		case <-w.ctx.Done():
			return
		case <-tmr.C:
			continue
		}
	}
}

func (w *writer) GetKey() uint64 {
	w.mut.RLock()
	defer w.mut.RUnlock()

	return w.currentKey
}

// incrementKey returns true if key changed
func (w *writer) incrementKey() bool {
	w.mut.Lock()
	defer w.mut.Unlock()

	prev := w.currentKey
	w.currentKey = w.store.GetNextKey(w.currentKey)
	// No need to update bookmark if nothing has changed.
	return prev != w.currentKey
}
