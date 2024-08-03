package imux

import (
	"os"
	"sync"
	"sync/atomic"
	"time"
)

func newWriteQueue(windowSize int, writeChannel chan *packet) *writeQueue {
	lock := &sync.Mutex{}

	return &writeQueue{
		elements:     make([]*packet, windowSize),
		readPointer:  0,
		writePointer: 0,
		ackPointer:   0,
		lock:         lock,
		cond:         sync.NewCond(lock),
		closed:       &atomic.Bool{},
		channel:      writeChannel,
	}
}

type writeQueue struct {
	elements     []*packet
	readPointer  int
	writePointer int
	ackPointer   int
	lock         *sync.Mutex
	cond         *sync.Cond
	closed       *atomic.Bool
	channel      chan *packet
}

func (q *writeQueue) ack(id uint32) {
	q.lock.Lock()
	defer q.lock.Unlock()

	currentAckToReadDistance := q.readPointer - q.ackPointer
	if currentAckToReadDistance < 0 {
		currentAckToReadDistance = q.readPointer + len(q.elements) - q.ackPointer
	}

	for i := 0; i < currentAckToReadDistance; i++ {
		currentPointer := (q.ackPointer + i) % len(q.elements)
		elem := q.elements[currentPointer]
		if elem.seqId == id {
			q.ackPointer = (currentPointer + 1) % len(q.elements)
			q.cond.Broadcast()
			return
		}
	}

	return
}

func (q *writeQueue) resetPointer() {
	q.lock.Lock()
	defer q.lock.Unlock()

	q.readPointer = q.ackPointer
	q.cond.Broadcast()
}

func (q *writeQueue) pop() *packet {
	for {
		if q.closed.Load() {
			return nil
		}
		if packet, ok := q.tryPop(); packet != nil || !ok {
			return packet
		}

		q.lock.Lock()
		q.cond.Wait()
		q.lock.Unlock()
	}
}

func (q *writeQueue) loop() bool {
	select {
	case packet, ok := <-q.channel:
		if !ok {
			return false
		}

		q.push(packet)
	}

	return true
}

func (q *writeQueue) tryPop() (*packet, bool) {
	q.lock.Lock()
	defer q.lock.Unlock()

	if q.closed.Load() {
		return nil, false
	}

	if q.readPointer == q.writePointer {
		return nil, true
	}

	element := q.elements[q.readPointer]
	q.readPointer = (q.readPointer + 1) % len(q.elements)

	return element, true
}

func (q *writeQueue) push(packet *packet) {
	for {
		if q.closed.Load() {
			return
		}
		if pushed := q.tryPush(packet); pushed {
			return
		}

		q.lock.Lock()
		q.cond.Wait()
		q.lock.Unlock()
	}
}

func (q *writeQueue) tryPush(packet *packet) bool {
	q.lock.Lock()
	defer q.lock.Unlock()

	writeToAckDistance := q.ackPointer - q.writePointer
	if q.writePointer >= q.ackPointer {
		writeToAckDistance = q.ackPointer + (len(q.elements) - q.writePointer)
	}

	windowHasSpace := writeToAckDistance > 1
	if !windowHasSpace {
		return false
	}

	q.elements[q.writePointer] = packet
	q.writePointer = (q.writePointer + 1) % len(q.elements)

	q.cond.Broadcast()

	return true
}

func (q *writeQueue) start() {
	go func() {
		for !q.closed.Load() {
			if shouldContinue := q.loop(); !shouldContinue {
				return
			}
		}
	}()
}

func (q *writeQueue) stop() {
	q.lock.Lock()
	defer q.lock.Unlock()

	q.cond.Broadcast()
}

func (q *writeQueue) waitUntilEmpty(deadline time.Time) error {
	q.lock.Lock()
	defer q.lock.Unlock()

	duration := max(deadline.Sub(time.Now()), 1)
	timer := time.NewTimer(duration)
	defer timer.Stop()

	go func() {
		select {
		case <-timer.C:
			q.cond.Broadcast()
		}
	}()

	for {
		if len(q.elements) == 0 {
			return nil
		}

		if time.Now().After(deadline) {
			return os.ErrDeadlineExceeded
		}

		q.cond.Wait()
	}
}
