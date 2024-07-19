package imux

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const PacketOverhead = 6

var ErrInvalidPacketSize = fmt.Errorf("received packet size is less than 6")

func readPacket(reader io.Reader, buff []byte) (*packet, error) {

	// read packet size
	n, err := io.ReadFull(reader, buff[0:2])
	if n < 2 {
		return nil, errors.New("unable to get packet size")
	}
	if err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint16(buff[0:2])
	if size < 6 {
		return nil, ErrInvalidPacketSize
	}

	// read sequence id
	n, err = io.ReadFull(reader, buff[2:4])
	if n < 2 {
		return nil, errors.New("unable to get seq id")
	}
	if err != nil {
		return nil, err
	}
	seqId := binary.LittleEndian.Uint16(buff[2:4])

	// read ack id
	n, err = io.ReadFull(reader, buff[4:6])
	if n < 2 {
		return nil, errors.New("unable to get ack id")
	}
	if err != nil {
		return nil, err
	}
	ackId := binary.LittleEndian.Uint16(buff[4:6])

	if hasData := size > 6; hasData {
		n, err = io.ReadFull(reader, buff[6:size])
		if n < 1 {
			return nil, errors.New("unable to get payload")
		}
		if err != nil {
			return nil, err
		}
	}

	return &packet{
		size:  size,
		seqId: seqId,
		ackId: ackId,
		buff:  buff,
	}, nil
}

func newAckPacket(buff []byte) *packet {
	binary.LittleEndian.PutUint16(buff[0:2], 6)
	binary.LittleEndian.PutUint16(buff[2:4], 0)
	binary.LittleEndian.PutUint16(buff[4:6], 0)

	return &packet{
		size:  6,
		ackId: 0,
		buff:  buff,
	}
}

func newPacket(buff []byte, seqId uint16, data []byte) *packet {
	binary.LittleEndian.PutUint16(buff[4:6], 0)
	binary.LittleEndian.PutUint16(buff[2:4], seqId)

	size := uint16(6)
	if data != nil && len(data) != 0 {
		copy(buff[6:len(data)+6], data[0:])
		size += uint16(len(data))
	}

	binary.LittleEndian.PutUint16(buff[0:2], size)

	return &packet{
		size:  size,
		seqId: seqId,
		ackId: 0,
		buff:  buff,
	}
}

type packet struct {
	size  uint16
	seqId uint16
	ackId uint16
	buff  []byte
}

func (p *packet) setAckId(ackId uint16) {
	p.ackId = ackId
	binary.LittleEndian.PutUint16(p.buff[4:6], ackId)
}

func (p *packet) data() []byte {
	return p.buff[6:p.size]
}

func (p *packet) isAck() bool {
	return p.size <= 6
}

func (p *packet) raw() []byte {
	return p.buff[0:p.size]
}
