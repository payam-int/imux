package imux

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestPushNextSeqId(t *testing.T) {
	packet := &packet{seqId: 0}
	queue := newReadQueue(10, 10)
	queue.push(packet)
	sorted := queue.trySort()

	assert.True(t, sorted)
}

func TestPushSecondNextSeqId(t *testing.T) {
	packet := &packet{seqId: 1}
	queue := newReadQueue(10, 10)
	queue.push(packet)
	sorted := queue.trySort()

	assert.False(t, sorted)
}

func TestPushInOppositeOrder(t *testing.T) {
	packet0 := &packet{seqId: 1}
	packet1 := &packet{seqId: 0}
	queue := newReadQueue(10, 10)
	queue.push(packet0)
	queue.push(packet1)

	sorted0 := queue.trySort()
	sorted1 := queue.trySort()

	assert.True(t, sorted0)
	assert.True(t, sorted1)
}

func TestTryPushWhenNoSpace(t *testing.T) {
	packet0 := &packet{seqId: 0}
	packet1 := &packet{seqId: 1}
	queue := newReadQueue(1, 1)
	push0 := queue.tryPush(packet0)
	push1 := queue.tryPush(packet1)

	assert.True(t, push0)
	assert.False(t, push1)
}

func TestTryPushWhenAfterNoSpace(t *testing.T) {
	packet0 := &packet{seqId: 0}
	packet1 := &packet{seqId: 1}
	queue := newReadQueue(1, 1)
	push0 := queue.tryPush(packet0)
	push1 := queue.tryPush(packet1)
	queue.trySort()
	push2 := queue.tryPush(packet1)

	assert.True(t, push0)
	assert.False(t, push1)
	assert.True(t, push2)
}
