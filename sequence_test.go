package imux

import (
	"github.com/stretchr/testify/assert"
	"math"
	"testing"
)

func TestIncUntil(t *testing.T) {
	seq := newSequence()
	expected := maxSeqWindow/2 + 1

	seq.incUntil(expected)
	got := seq.get()

	assert.Equal(t, expected, got)
}

func TestIncUntilLargerThanWindow(t *testing.T) {
	seq := newSequence()
	expected := uint16(math.MaxUint16)

	inc := maxSeqWindow + 1
	seq.incUntil(inc)

	got := seq.get()

	assert.Equal(t, expected, got)
}
