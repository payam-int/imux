package imux

import (
	"sync"
	"sync/atomic"
)

func newReadQueue(packetChan chan *packet, windowSize int, poolSize int) *readQueue {
	lock := &sync.Mutex{}
	return &readQueue{
		packetChan: packetChan,
		window:     make([]*packet, poolSize*windowSize),
		lock:       lock,
		cond:       sync.NewCond(lock),
		closed:     &atomic.Bool{},
	}
}

type readQueue struct {
	packetChan chan *packet
	window     []*packet
	lock       *sync.Mutex
	cond       *sync.Cond
	closed     *atomic.Bool
	nextSeqId  uint32
	pointer    int
}

func (r *readQueue) start() {
	go func() {
		for {
			r.lock.Lock()

			if r.closed.Load() {
				close(r.packetChan)
				return
			}

			next := r.tryPopSorted()
			if next != nil {
				r.cond.Broadcast()
			} else {
				r.cond.Wait()
			}

			r.lock.Unlock()

			if next != nil {
				r.packetChan <- next
			}
		}
	}()
}

func (r *readQueue) stop() {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.closed.Store(true)
	r.cond.Broadcast()
}

func (r *readQueue) tryPopSorted() *packet {
	currentPacket := r.window[r.pointer]
	if currentPacket == nil {
		return nil
	}

	r.window[r.pointer] = nil
	r.pointer = (r.pointer + 1) % len(r.window)
	r.nextSeqId += 1

	return currentPacket
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
