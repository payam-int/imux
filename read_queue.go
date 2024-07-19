package imux

import (
	"sync"
	"sync/atomic"
)

func newReadQueue(windowSize int, poolSize int) *readQueue {
	lock := &sync.Mutex{}
	return &readQueue{
		orderedPackets: make(chan *packet, windowSize),
		window:         make([]*packet, poolSize*windowSize),
		lock:           lock,
		cond:           sync.NewCond(lock),
		closed:         &atomic.Bool{},
	}
}

type readQueue struct {
	orderedPackets chan *packet
	window         []*packet
	lock           *sync.Mutex
	cond           *sync.Cond
	closed         *atomic.Bool
	nextSeqId      uint16
	pointer        int
}

func (r *readQueue) start() {
	go func() {
		for r.closed.Store(false); !r.closed.Load(); {
			if sorted := r.trySort(); !sorted {
				r.lock.Lock()
				r.cond.Wait()
				r.lock.Unlock()
			}
		}
	}()
}

func (r *readQueue) stop() {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.closed.Store(true)
	r.cond.Broadcast()

	close(r.orderedPackets)
}

func (r *readQueue) trySort() bool {
	r.lock.Lock()

	packet := r.window[r.pointer]
	if packet == nil {
		r.lock.Unlock()
		return false
	}

	r.window[r.pointer] = nil
	r.pointer = (r.pointer + 1) % len(r.window)
	r.nextSeqId += 1
	r.cond.Broadcast()
	r.lock.Unlock()

	r.orderedPackets <- packet

	return true
}

func (r *readQueue) push(packet *packet) {
	for {
		if pushed := r.tryPush(packet); pushed {
			return
		} else {
			r.lock.Lock()
			r.cond.Wait()
			r.lock.Unlock()
		}
	}
}

func (r *readQueue) tryPush(packet *packet) bool {
	r.lock.Lock()
	defer r.lock.Unlock()

	if r.closed.Load() {
		return true
	}

	distanceToCurrentSeqId := int(packet.seqId - r.nextSeqId)
	windowHasSpace := distanceToCurrentSeqId < len(r.window)
	if !windowHasSpace {
		return false
	}

	newElementPosition := (r.pointer + distanceToCurrentSeqId) % len(r.window)
	r.window[newElementPosition] = packet

	r.cond.Broadcast()

	return true
}

func (r *readQueue) getReadChan() <-chan *packet {
	return r.orderedPackets
}
