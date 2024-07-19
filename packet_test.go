package imux

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewPacket(t *testing.T) {
	buff := make([]byte, 50)
	payload := []byte{0xFF, 0xFF}
	expected := []byte{0x08, 0, 0xA, 0, 0, 0, 0xFF, 0xFF}

	packet := newPacket(buff, 10, payload)
	got := packet.raw()

	assert.Equal(t, expected, got)
}

func TestReadPacket(t *testing.T) {
	buff := make([]byte, 50)
	reader := bytes.NewReader([]byte{0x08, 0, 0xA, 0, 0x9, 0, 0xFF, 0xFF})

	expectedSeqId := uint16(10)
	expectedAckId := uint16(9)
	expectedData := []byte{0xFF, 0xFF}

	packet, err := readPacket(reader, buff)

	assert.NoError(t, err)
	assert.Equal(t, expectedData, packet.data())
	assert.Equal(t, expectedSeqId, packet.seqId)
	assert.Equal(t, expectedAckId, packet.ackId)
}

func TestSetAckId(t *testing.T) {
	buff := make([]byte, 50)
	payload := []byte{0xFF, 0xFF}
	expected := []byte{8, 0, 10, 0, 8, 0, 0xFF, 0xFF}

	packet := newPacket(buff, 10, payload)

	packet.setAckId(8)

	got := packet.raw()

	assert.Equal(t, expected, got)
}
