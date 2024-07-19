package imux

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestPush(t *testing.T) {
	packet := &packet{size: 10}
	queue := newWriteQueue(10, nil)
	pushed := queue.tryPush(packet)

	assert.True(t, pushed)
}

func TestPushPop(t *testing.T) {
	packet := &packet{size: 10}
	queue := newWriteQueue(10, nil)
	pushed := queue.tryPush(packet)
	got, didPop := queue.tryPop()

	assert.True(t, pushed)
	assert.True(t, didPop)
	assert.Equal(t, packet, got)
}

func TestPushPush(t *testing.T) {
	packet0 := &packet{size: 10}
	packet1 := &packet{size: 10, seqId: 1}
	queue := newWriteQueue(2, nil)
	queue.tryPush(packet0)
	pushed := queue.tryPush(packet1)

	assert.False(t, pushed)
}

func TestPushAckPush(t *testing.T) {
	packet0 := &packet{size: 10}
	packet1 := &packet{size: 10, seqId: 1}
	queue := newWriteQueue(2, nil)
	queue.tryPush(packet0)
	queue.tryPop()
	queue.ack(0)
	pushed := queue.tryPush(packet1)

	assert.True(t, pushed)
}

func TestPushAckResetPop(t *testing.T) {
	packet := &packet{size: 10}
	queue := newWriteQueue(2, nil)
	queue.tryPush(packet)
	queue.tryPop()
	queue.ack(0)
	queue.resetPointer()
	got, _ := queue.tryPop()

	assert.Nil(t, got)
}
