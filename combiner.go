package imux

import (
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// NewCombiner creates a new Combiner instance based on the provided configuration.
// To the Combiner, provide it with a sufficient number of connections using the SetConn method
// and When a connection closes, replace it with a new connection having the same TunnelId.
//
// To receive handle connection close event, provide an OnError function within the CombinerConfig.
func NewCombiner(config CombinerConfig) (*Combiner, error) {
	if config.PacketSize > math.MaxUint32-HeaderSize {
		return nil, fmt.Errorf("packet size must be less than 42e8")
	}

	if config.PoolSize*config.WindowSize > math.MaxUint32 {
		return nil, fmt.Errorf("poolSize * windowSize must be less than 42e8")
	}

	onError := func(tunnelId TunnelId, conn net.Conn, err error) {}
	if config.OnError != nil {
		onError = config.OnError
	}

	bufferPool := &sync.Pool{
		New: func() any {
			return make([]byte, config.PacketSize+HeaderSize)
		},
	}

	tunnels := make([]*tunnel, config.PoolSize)
	packetChan := make(chan *packet, config.WindowSize)
	sorterQueue := newReadQueue(packetChan, config.WindowSize, config.PoolSize)
	writeChan := make(chan *packet, config.PoolSize)
	ackDelay := max(config.WindowSize/2, 1)

	for i := 0; i < config.PoolSize; i++ {
		tunWriteQueue := newWriteQueue(config.WindowSize, writeChan)
		tunnels[i] = newTunnel(i, sorterQueue, tunWriteQueue, bufferPool, onError, config.AckTimeout, ackDelay)
	}

	combiner := &Combiner{
		tag:          config.Tag,
		packetSize:   config.PacketSize,
		packetChan:   packetChan,
		poolSize:     config.PoolSize,
		onConnError:  onError,
		sorterQueue:  sorterQueue,
		tunnels:      tunnels,
		seqLock:      &sync.Mutex{},
		readLock:     &sync.Mutex{},
		writeLock:    &sync.Mutex{},
		bufferPool:   bufferPool,
		writeChan:    writeChan,
		closed:       &atomic.Bool{},
		closing:      &atomic.Bool{},
		closeTimeout: config.CloseTimeOut,
	}

	if err := combiner.start(); err != nil {
		_ = combiner.Close()
		return nil, err
	}

	return combiner, nil
}

type OnConnErrorFunc = func(tunnelId TunnelId, conn net.Conn, err error)

// CombinerConfig holds configuration parameters for the Combiner.
type CombinerConfig struct {
	// Tag is an arbitrary name for the Combiner instance
	Tag string

	// PoolSize specifies the  number of underlying connections.
	PoolSize int

	// WindowSize defines the maximum number of unacknowledged packets.
	WindowSize int

	// PacketSize sets the size of data packets to be sent.
	PacketSize int

	// OnError is a callback function invoked when an error occurs.
	OnError OnConnErrorFunc

	// AckTimeout is the duration to wait for a packet to send before sending an empty packet with ack.
	AckTimeout time.Duration

	// Maximum deadline for closing a connection
	CloseTimeOut time.Duration
}

// Combiner is a net.Conn implementation that distributes data across multiple underlying connections
// The overall throughput is limited by the slowest connection. For a pool size of n and the slowest
// connection throughput of s, the maximum achievable throughput is n * s.
type Combiner struct {
	packetSize     int
	poolSize       int
	sequenceId     uint32
	seqLock        *sync.Mutex
	readLock       *sync.Mutex
	writeLock      *sync.Mutex
	tunnels        []*tunnel
	readDeadline   *time.Time
	writeDeadline  *time.Time
	sorterQueue    *readQueue
	packetChan     chan *packet
	readingPacket  *packet
	readingPointer int
	bufferPool     *sync.Pool
	writeChan      chan *packet
	closed         *atomic.Bool
	closing        *atomic.Bool
	closeTimeout   time.Duration
	tag            string
	onConnError    OnConnErrorFunc
}

// LocalAddr returns an empty net.TCPAddr to satisfy the net.Conn interface.
func (c *Combiner) LocalAddr() net.Addr {
	return &net.TCPAddr{}
}

// RemoteAddr returns an empty net.TCPAddr to satisfy the net.Conn interface.
func (c *Combiner) RemoteAddr() net.Addr {
	return &net.TCPAddr{}
}

// Read implements the net.Conn interface and behaves identically to net.Conn.Read.
func (c *Combiner) Read(b []byte) (int, error) {
	if c.closed.Load() {
		return 0, io.ErrClosedPipe
	}

	c.readLock.Lock()
	defer c.readLock.Unlock()

	deadline := c.readDeadline
	n := 0
	for {
		if c.readingPacket == nil {
			if c.closed.Load() {
				return n, io.ErrClosedPipe
			}

			gotPacket, ok, err := c.readUntilDeadline(deadline)
			if err != nil {
				return n, err
			}
			if !ok {
				// it happens when user is still waiting on read but the socket is somehow closed
				return n, io.ErrClosedPipe
			}

			c.readingPacket = gotPacket
			c.readingPointer = 0
		}

		if c.readingPacket.isClose() {
			_ = c.Close()
			break
		}

		unreadPayload := c.readingPacket.data()[c.readingPointer:]
		readPayloadSize := min(len(b)-n, len(unreadPayload))
		copy(b[n:], unreadPayload[:readPayloadSize])

		c.readingPointer += readPayloadSize
		n += readPayloadSize

		if readPayloadSize == len(unreadPayload) {
			c.bufferPool.Put(c.readingPacket.buff)
			c.readingPacket = nil
		}

		if n >= len(b) {
			break
		}
	}

	return n, nil
}

func (c *Combiner) readUntilDeadline(deadline *time.Time) (packet *packet, ok bool, err error) {
	if deadline == nil {
		select {
		case packet, ok = <-c.packetChan:
		}

		return
	}

	idleDelay := time.NewTimer(time.Until(*deadline))
	defer idleDelay.Stop()

	select {
	case packet, ok = <-c.packetChan:
	case <-idleDelay.C:
		err = os.ErrDeadlineExceeded
		return
	}

	return
}

// Write implements the net.Conn interface and behaves identically to net.Conn.Write.
func (c *Combiner) Write(b []byte) (n int, err error) {
	if c.closing.Load() {
		return 0, io.ErrClosedPipe
	}

	c.writeLock.Lock()
	defer c.writeLock.Unlock()

	deadline := c.writeDeadline
	pointerStart := 0

	for {
		if c.closing.Load() {
			return 0, io.ErrClosedPipe
		}

		pointerEnd := pointerStart + min(c.packetSize, len(b)-pointerStart)
		packetData := b[pointerStart:pointerEnd]

		buff := c.bufferPool.Get().([]byte)
		seqId := c.nextSequenceId()

		nextPacket := newPacket(buff, seqId, packetData)

		if err := c.writeUntilDeadline(nextPacket, deadline); err != nil {
			return pointerStart, err
		}

		pointerStart = pointerEnd
		if pointerEnd == len(b) {
			return pointerStart, nil
		}
	}
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

// Close implements the net.Conn interface and behaves identically to net.Conn.Close.
func (c *Combiner) Close() error {
	if c.closing.Load() {
		return nil
	}

	c.closing.Store(true)

	buff := c.bufferPool.Get().([]byte)
	closePacket := newClosePacket(buff)

	c.writeLock.Lock()
	defer c.writeLock.Unlock()

	closeUntil := time.Now().Add(c.closeTimeout)
	_ = c.writeUntilDeadline(closePacket, &closeUntil)
	close(c.writeChan)

	c.sorterQueue.stop()

	for _, tun := range c.tunnels {
		tun.stop(closeUntil)
	}

	return nil
}

func (c *Combiner) start() error {
	c.sorterQueue.start()

	for _, tunnel := range c.tunnels {
		tunnel.start()
	}

	return nil
}

// SetDeadline implements the net.Conn interface and behaves identically to net.Conn.SetDeadline.
func (c *Combiner) SetDeadline(t time.Time) error {
	_ = c.SetReadDeadline(t)
	_ = c.SetWriteDeadline(t)

	return nil
}

// SetReadDeadline implements the net.Conn interface and behaves identically to net.Conn.SetReadDeadline.
func (c *Combiner) SetReadDeadline(t time.Time) error {
	c.readDeadline = &t
	return nil
}

// SetWriteDeadline implements the net.Conn interface and behaves identically to net.Conn.SetWriteDeadline.
func (c *Combiner) SetWriteDeadline(t time.Time) error {
	c.writeDeadline = &t
	return nil
}

// SetConn sets the underlying connection for the specified tunnelId.
func (c *Combiner) SetConn(tunnelId TunnelId, conn net.Conn) {
	c.tunnels[tunnelId].bind(conn)
}

// DeleteConn removes the underlying connection associated with the specified tunnelId.
func (c *Combiner) DeleteConn(tunnelId TunnelId) {
	c.tunnels[tunnelId].unbind()
}

// GetActiveConnections returns the number of active underlying connections.
func (c *Combiner) GetActiveConnections() int {
	activeConnections := 0
	for _, tunnel := range c.tunnels {
		if tunnel.hasConnection() {
			activeConnections += 1
		}
	}

	return activeConnections
}

// GetPoolSize returns the maximum number of underlying connections.
func (c *Combiner) GetPoolSize() int {
	return c.poolSize
}

func (c *Combiner) nextSequenceId() uint32 {
	c.seqLock.Lock()
	defer c.seqLock.Unlock()

	seqId := c.sequenceId
	c.sequenceId += 1

	return seqId
}
