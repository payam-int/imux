package imux

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewPacket(t *testing.T) {
	buff := make([]byte, 50)
	payload := []byte{0xFF, 0xFF}
	expected := []byte{0x10, 0, 0, 0, 0, 0, 0xA, 0, 0, 0, 0, 0, 0, 0, 0xFF, 0xFF}

	packet := newPacket(buff, 10, payload)
	got := packet.raw()

	assert.Equal(t, expected, got)
}

func TestReadPacket(t *testing.T) {
	buff := make([]byte, 50)
	reader := bytes.NewReader([]byte{0x10, 0, 0, 0, 0, 0, 0xA, 0, 0, 0, 0x9, 0, 0, 0, 0xFF, 0xFF})

	expectedSeqId := uint32(10)
	expectedAckId := uint32(9)
	expectedData := []byte{0xFF, 0xFF}

	gotPacket, err := readPacket(reader, buff)

	assert.NoError(t, err)
	assert.Equal(t, expectedData, gotPacket.data())
	assert.Equal(t, expectedSeqId, gotPacket.seqId)
	assert.Equal(t, expectedAckId, gotPacket.ackId)
}

func TestSetAckId(t *testing.T) {
	buff := make([]byte, 50)
	payload := []byte{0xFF, 0xFF}
	expected := []byte{0x10, 0, 0, 0, 0, 0, 0x0A, 0, 0, 0, 0x08, 0, 0, 0, 0xFF, 0xFF}

	packet := newPacket(buff, 10, payload)

	packet.setAckId(8)

	got := packet.raw()

	assert.Equal(t, expected, got)
}
