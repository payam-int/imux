package imux

import (
	"math"
	"sync"
)

var maxSeqWindow = uint16(math.MaxUint16 / 3)

func newSequence() *sequence {
	return &sequence{
		id:   math.MaxUint16,
		lock: &sync.Mutex{},
	}
}

type sequence struct {
	id   uint16
	lock *sync.Mutex
}

func (s *sequence) inc() uint16 {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.id += 1

	return s.id
}

func (s *sequence) incUntil(nextId uint16) (bool, uint16) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if (s.id <= nextId) && (nextId-s.id) < maxSeqWindow ||
		(s.id > nextId) && (math.MaxUint16-s.id+nextId) < maxSeqWindow {

		s.id = nextId

		return false, nextId
	}

	return true, s.id
}

func (s *sequence) get() uint16 {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.id
}
