package imux

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	CmdNoOp = iota
	CmdAck  = iota
	CmdClose
)

const (
	SizeLen  = 4
	CmdLen   = 2
	AckIdLen = 4
	SeqIdLen = 4
)

const HeaderSize = SizeLen + CmdLen + SeqIdLen + AckIdLen

var ErrInvalidPacketSize = fmt.Errorf("received packet size is less than 6")

func readPacket(reader io.Reader, buff []byte) (*packet, error) {
	// read packet size
	sizeBuff := buff[:SizeLen]
	n, err := io.ReadFull(reader, sizeBuff)
	if n < SizeLen {
		return nil, errors.New("unable to get packet size")
	}
	if err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(sizeBuff)
	if size < HeaderSize {
		return nil, ErrInvalidPacketSize
	}

	// read sequence id
	cmdBuff := buff[SizeLen : SizeLen+CmdLen]
	n, err = io.ReadFull(reader, cmdBuff)
	if n < CmdLen {
		return nil, errors.New("unable to get cmd")
	}
	if err != nil {
		return nil, err
	}
	cmd := binary.LittleEndian.Uint16(cmdBuff)

	// read sequence id
	seqIdBuff := buff[SizeLen+CmdLen : SizeLen+CmdLen+SeqIdLen]
	n, err = io.ReadFull(reader, seqIdBuff)
	if n < SeqIdLen {
		return nil, errors.New("unable to get seq id")
	}
	if err != nil {
		return nil, err
	}
	seqId := binary.LittleEndian.Uint32(seqIdBuff)

	// read ack id
	ackIdBuff := buff[SizeLen+CmdLen+SeqIdLen : SizeLen+CmdLen+SeqIdLen+AckIdLen]
	n, err = io.ReadFull(reader, ackIdBuff)
	if n < AckIdLen {
		return nil, errors.New("unable to get ack id")
	}
	if err != nil {
		return nil, err
	}
	ackId := binary.LittleEndian.Uint32(ackIdBuff)

	// read payload
	if hasData := size > HeaderSize; hasData {
		n, err = io.ReadFull(reader, buff[HeaderSize:size])
		if n < 1 {
			return nil, errors.New("unable to get data")
		}
		if err != nil {
			return nil, err
		}
	}

	return &packet{
		size:  size,
		cmd:   cmd,
		seqId: seqId,
		ackId: ackId,
		buff:  buff,
	}, nil
}

func newAckPacket(buff []byte) *packet {
	binary.LittleEndian.PutUint32(buff[0:SizeLen], HeaderSize)
	binary.LittleEndian.PutUint16(buff[SizeLen:SizeLen+CmdLen], CmdAck)
	binary.LittleEndian.PutUint32(buff[SizeLen+CmdLen:SizeLen+CmdLen+SeqIdLen], 0)
	binary.LittleEndian.PutUint32(buff[SizeLen+CmdLen+SeqIdLen:SizeLen+CmdLen+SeqIdLen+AckIdLen], 0)

	return &packet{
		size:  HeaderSize,
		cmd:   CmdAck,
		ackId: 0,
		buff:  buff,
	}
}

func newClosePacket(buff []byte) *packet {
	binary.LittleEndian.PutUint32(buff[0:SizeLen], HeaderSize)
	binary.LittleEndian.PutUint16(buff[SizeLen:SizeLen+CmdLen], CmdClose)
	binary.LittleEndian.PutUint32(buff[SizeLen+CmdLen:SizeLen+CmdLen+SeqIdLen], 0)
	binary.LittleEndian.PutUint32(buff[SizeLen+CmdLen+SeqIdLen:SizeLen+CmdLen+SeqIdLen+AckIdLen], 0)

	return &packet{
		size:  HeaderSize,
		cmd:   CmdClose,
		ackId: 0,
		buff:  buff,
	}
}

func newPacket(buff []byte, seqId uint32, data []byte) *packet {
	binary.LittleEndian.PutUint16(buff[SizeLen:SizeLen+CmdLen], CmdNoOp)
	binary.LittleEndian.PutUint32(buff[SizeLen+CmdLen:SizeLen+CmdLen+SeqIdLen], seqId)
	binary.LittleEndian.PutUint32(buff[SizeLen+CmdLen+SeqIdLen:SizeLen+CmdLen+SeqIdLen+AckIdLen], 0)

	size := uint32(HeaderSize)
	if data != nil && len(data) != 0 {
		copy(buff[HeaderSize:len(data)+HeaderSize], data[0:])
		size += uint32(len(data))
	}

	binary.LittleEndian.PutUint32(buff[0:SizeLen], size)

	return &packet{
		size:  size,
		cmd:   CmdNoOp,
		seqId: seqId,
		ackId: 0,
		buff:  buff,
	}
}

type packet struct {
	size  uint32
	cmd   uint16
	seqId uint32
	ackId uint32
	buff  []byte
}

func (p *packet) setAckId(ackId uint32) {
	p.ackId = ackId
	binary.LittleEndian.PutUint32(p.buff[SizeLen+CmdLen+SeqIdLen:SizeLen+CmdLen+SeqIdLen+AckIdLen], ackId)
}

func (p *packet) isAck() bool {
	return p.cmd == CmdAck
}

func (p *packet) isClose() bool {
	return p.cmd == CmdClose
}

func (p *packet) data() []byte {
	return p.buff[HeaderSize:p.size]
}

func (p *packet) raw() []byte {
	return p.buff[0:p.size]
}
