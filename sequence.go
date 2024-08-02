package imux

import (
	"math"
	"sync"
)

var maxSeqWindow = uint32(math.MaxUint32 / 3)

func newSequence() *sequence {
	return &sequence{
		id:   math.MaxUint32,
		lock: &sync.Mutex{},
	}
}

type sequence struct {
	id   uint32
	lock *sync.Mutex
}

func (s *sequence) inc() uint32 {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.id += 1

	return s.id
}

func (s *sequence) incUntil(nextId uint32) (bool, uint32) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if (s.id <= nextId) && (nextId-s.id) < maxSeqWindow ||
		(s.id > nextId) && (math.MaxUint32-s.id+nextId) < maxSeqWindow {

		s.id = nextId

		return false, nextId
	}

	return true, s.id
}

func (s *sequence) get() uint32 {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.id
}
