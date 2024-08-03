package imux

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestPushNextSeqId(t *testing.T) {
	expected := &packet{seqId: 0}

	queue := newReadQueue(nil, 10, 10)
	queue.push(expected)

	got := queue.tryPopSorted()

	assert.Equal(t, expected, got)
}

func TestPushSecondNextSeqId(t *testing.T) {
	given := &packet{seqId: 1}

	queue := newReadQueue(nil, 10, 10)
	queue.push(given)

	got := queue.tryPopSorted()

	assert.Nil(t, got)
}

func TestPushInOppositeOrder(t *testing.T) {
	expected1, expected0 := &packet{seqId: 1}, &packet{seqId: 0}

	queue := newReadQueue(nil, 10, 10)
	queue.push(expected1)
	queue.push(expected0)

	got0, got1 := queue.tryPopSorted(), queue.tryPopSorted()

	assert.Equal(t, expected0, got0)
	assert.Equal(t, expected1, got1)
}

func TestTryPushWhenNoSpace(t *testing.T) {
	packet0 := &packet{seqId: 0}
	packet1 := &packet{seqId: 1}

	queue := newReadQueue(nil, 1, 1)

	push0, push1 := queue.tryPush(packet0), queue.tryPush(packet1)

	assert.True(t, push0)
	assert.False(t, push1)
}

func TestTryPushWhenAfterNoSpace(t *testing.T) {
	packet0 := &packet{seqId: 0}
	packet1 := &packet{seqId: 1}

	queue := newReadQueue(nil, 1, 1)
	push0, push1 := queue.tryPush(packet0), queue.tryPush(packet1)

	queue.tryPopSorted()

	push2 := queue.tryPush(packet1)

	assert.True(t, push0)
	assert.False(t, push1)
	assert.True(t, push2)
}
