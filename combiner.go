package imux

import (
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

func NewCombiner(config CombinerConfig) (*Combiner, error) {
	if config.PacketSize > 10000 {
		return nil, fmt.Errorf("packet size must be less than 10000")
	}

	if config.PoolSize*config.WindowSize > 20000 {
		return nil, fmt.Errorf("poolSize * windowSize must be less than 20000")
	}

	onConnErr := func(tunnelId TunnelId, conn net.Conn, err error) {}
	if config.OnError != nil {
		onConnErr = config.OnError
	}

	bufferPool := &sync.Pool{
		New: func() any {
			return make([]byte, config.PacketSize+PacketOverhead)
		},
	}
	tunnels := make([]*tunnel, config.PoolSize)
	sorterReadQueue := newReadQueue(config.WindowSize, config.PoolSize)
	writeChan := make(chan *packet, config.PoolSize)
	ackDelay := max(config.WindowSize/2, 1)

	for i := 0; i < config.PoolSize; i++ {
		writeQueue := newWriteQueue(config.WindowSize, writeChan)
		tunnels[i] = newTunnel(i, sorterReadQueue, writeQueue, bufferPool, onConnErr, config.AckTimeout, ackDelay)
	}

	combiner := &Combiner{
		tag:         config.Tag,
		packetSize:  config.PacketSize,
		poolSize:    config.PoolSize,
		onConnError: onConnErr,
		sorterQueue: sorterReadQueue,
		tunnels:     tunnels,
		lock:        &sync.Mutex{},
		bufferPool:  bufferPool,
		writeChan:   writeChan,
	}

	if err := combiner.start(); err != nil {
		_ = combiner.Close()
		return nil, err
	}

	return combiner, nil
}

type OnConnErrorFunc = func(tunnelId TunnelId, conn net.Conn, err error)

type CombinerConfig struct {
	Tag        string
	PoolSize   int
	WindowSize int
	PacketSize int
	OnError    OnConnErrorFunc
	AckTimeout time.Duration
}

type Combiner struct {
	packetSize    int
	poolSize      int
	lock          *sync.Mutex
	onConnError   OnConnErrorFunc
	tunnels       []*tunnel
	readDeadline  *time.Time
	writeDeadline *time.Time
	sorterQueue   *readQueue
	packetReading *packet
	packetPointer int
	sequenceId    uint16
	bufferPool    *sync.Pool
	writeChan     chan *packet
	closed        bool
	tag           string
}

func (c *Combiner) LocalAddr() net.Addr {
	return &net.TCPAddr{}
}

func (c *Combiner) RemoteAddr() net.Addr {
	return &net.TCPAddr{}
}

func (c *Combiner) readUntilDeadline(deadline *time.Time) (packet *packet, ok bool, err error) {
	if deadline == nil {
		select {
		case packet, ok = <-c.sorterQueue.getReadChan():
		}

		return
	}

	idleDelay := time.NewTimer(time.Until(*deadline))
	defer idleDelay.Stop()

	select {
	case packet, ok = <-c.sorterQueue.getReadChan():
	case <-idleDelay.C:
		err = os.ErrDeadlineExceeded
		return
	}

	return
}

func (c *Combiner) Read(b []byte) (int, error) {
	if c.closed {
		return 0, io.ErrClosedPipe
	}

	deadline := c.readDeadline
	readBuffPointer := 0
	for {
		if c.packetReading == nil {
			if c.closed {
				return 0, io.ErrClosedPipe
			}

			packet, ok, err := c.readUntilDeadline(deadline)
			if err != nil && readBuffPointer == 0 {
				return 0, err
			}

			if !ok {
				c.closed = true
			}

			c.packetReading = packet
			c.packetPointer = 0
		}

		unreadPayload := c.packetReading.data()[c.packetPointer:]
		endPointer := min(len(b)-readBuffPointer, len(unreadPayload))
		copy(b[readBuffPointer:], unreadPayload[:endPointer])

		c.packetPointer += endPointer
		readBuffPointer += endPointer

		if endPointer == len(unreadPayload) {
			c.bufferPool.Put(c.packetReading.buff)
			c.packetReading = nil
		}

		if readBuffPointer >= len(b) {
			break
		}
	}

	return readBuffPointer, nil
}

func (c *Combiner) writeUntilDeadline(packet *packet, deadline *time.Time) error {
	if deadline == nil {
		select {
		case c.writeChan <- packet:
		}

		return nil
	}

	idleDelay := time.NewTimer(time.Until(*deadline))
	defer idleDelay.Stop()

	select {
	case c.writeChan <- packet:
	case <-idleDelay.C:
		return os.ErrDeadlineExceeded
	}

	return nil
}

func (c *Combiner) Write(b []byte) (n int, err error) {
	if c.closed {
		return 0, io.ErrClosedPipe
	}

	deadline := c.writeDeadline
	pointerStart := 0

	for {
		if c.closed {
			return 0, io.ErrClosedPipe
		}

		pointerEnd := pointerStart + min(c.packetSize, len(b)-pointerStart)
		packetData := b[pointerStart:pointerEnd]

		buff := c.bufferPool.Get().([]byte)
		seqId := c.nextSequenceId()

		packet := newPacket(buff, seqId, packetData)

		if err := c.writeUntilDeadline(packet, deadline); err != nil {
			return pointerStart, err
		}

		pointerStart = pointerEnd
		if pointerEnd == len(b) {
			return pointerStart, nil
		}
	}
}

func (c *Combiner) Close() error {
	c.sorterQueue.stop()

	for _, tunnel := range c.tunnels {
		tunnel.stop()
	}

	close(c.writeChan)

	return nil
}

func (c *Combiner) start() error {
	c.sorterQueue.start()

	for _, tunnel := range c.tunnels {
		tunnel.start()
	}

	return nil
}

func (c *Combiner) SetDeadline(t time.Time) error {
	_ = c.SetReadDeadline(t)
	_ = c.SetWriteDeadline(t)

	return nil
}

func (c *Combiner) SetReadDeadline(t time.Time) error {
	c.readDeadline = &t
	return nil
}

func (c *Combiner) SetWriteDeadline(t time.Time) error {
	c.writeDeadline = &t
	return nil
}

func (c *Combiner) SetConn(tunnelId TunnelId, conn net.Conn) error {
	return c.tunnels[tunnelId].bind(conn)
}

func (c *Combiner) DeleteConn(tunnelId TunnelId) error {
	return c.tunnels[tunnelId].unbind()
}

func (c *Combiner) GetActiveConnections() int {
	activeConnections := 0
	for _, tunnel := range c.tunnels {
		if tunnel.hasConnection() {
			activeConnections += 1
		}
	}

	return activeConnections
}

func (c *Combiner) GetPoolSize() int {
	return c.poolSize
}

func (c *Combiner) nextSequenceId() uint16 {
	c.lock.Lock()
	defer c.lock.Unlock()

	seqId := c.sequenceId
	c.sequenceId += 1

	return seqId
}
