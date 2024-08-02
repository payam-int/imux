package imux

import (
	"math"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const AckTimerDuration = 10 * time.Millisecond

type TunnelId = int

func newTunnel(
	tunnelId TunnelId,
	readQueue *readQueue,
	writeQueue *writeQueue,
	bufferPool *sync.Pool,
	onConnError OnConnErrorFunc,
	ackTimeout time.Duration,
	ackDelay int,
) *tunnel {
	connLock := &sync.Mutex{}

	packetSentAt := &atomic.Pointer[time.Time]{}
	currentTime := time.Now()
	packetSentAt.Store(&currentTime)

	return &tunnel{
		conn:          nil,
		connCond:      sync.NewCond(connLock),
		connLock:      connLock,
		writeLock:     &sync.Mutex{},
		readQueue:     readQueue,
		writeQueue:    writeQueue,
		bufferPool:    bufferPool,
		onConnError:   onConnError,
		lastReadSeqId: newSequence(),
		tunnelId:      tunnelId,
		closed:        &atomic.Bool{},
		ackTimeout:    ackTimeout,
		ackDelay:      ackDelay,
		packetSentAt:  packetSentAt,
		delayedAcks:   &atomic.Int64{},
	}
}

type tunnel struct {
	tunnelId      TunnelId
	conn          net.Conn
	connCond      *sync.Cond
	connLock      *sync.Mutex
	writeLock     *sync.Mutex
	closed        *atomic.Bool
	readQueue     *readQueue
	writeQueue    *writeQueue
	bufferPool    *sync.Pool
	onConnError   OnConnErrorFunc
	lastReadSeqId *sequence
	ackTimeout    time.Duration
	packetSentAt  *atomic.Pointer[time.Time]
	ackDelay      int
	delayedAcks   *atomic.Int64
}

func (t *tunnel) read() {
	conn := t.waitForConn()

	buff := t.bufferPool.Get().([]byte)
	gotPacket, err := readPacket(conn, buff)
	if err != nil {
		t.onConnError(t.tunnelId, conn, err)
		return
	}

	if gotPacket.isAck() {
		if gotPacket.ackId == math.MaxUint32 {
			return
		}

		t.writeQueue.ack(gotPacket.ackId)

		return
	}

	if repeated, _ := t.lastReadSeqId.incUntil(gotPacket.seqId); repeated {
		return
	}

	t.ack()
	t.writeQueue.ack(gotPacket.ackId)
	t.delayedAcks.Add(1)

	t.readQueue.push(gotPacket)
}

func (t *tunnel) write() {
	packet := t.writeQueue.pop()
	t.writePacket(packet)
}
func (t *tunnel) writePacket(packet *packet) {
	conn := t.waitForConn()

	t.delayedAcks.Store(0)
	packet.setAckId(t.lastReadSeqId.get())

	t.writeLock.Lock()
	_, err := conn.Write(packet.raw())
	t.writeLock.Unlock()
	if err != nil {
		t.onConnError(t.tunnelId, conn, err)
		return
	}

	ackSendTime := time.Now()
	t.packetSentAt.Store(&ackSendTime)
	t.bufferPool.Put(packet.buff)

	return
}

func (t *tunnel) bind(conn net.Conn) {
	t.connLock.Lock()
	defer t.connLock.Unlock()

	t.conn = conn
	t.connCond.Broadcast()
}

func (t *tunnel) unbind() {
	t.connLock.Lock()
	defer t.connLock.Unlock()

	t.conn = nil
	t.writeQueue.resetPointer()
	t.connCond.Broadcast()
}

func (t *tunnel) hasConnection() bool {
	t.connLock.Lock()
	defer t.connLock.Unlock()

	return t.conn != nil
}

func (t *tunnel) waitForConn() net.Conn {
	t.connLock.Lock()
	defer t.connLock.Unlock()

	for {
		if conn := t.conn; conn != nil {
			return conn
		}
		t.connCond.Wait()
	}
}

func (t *tunnel) ack() {
	if int(t.delayedAcks.Load()) < t.ackDelay &&
		t.packetSentAt.Load().Add(t.ackTimeout).After(time.Now()) {
		return
	}

	buff := t.bufferPool.Get().([]byte)
	ackPacket := newAckPacket(buff)

	t.writePacket(ackPacket)
}

func (t *tunnel) start() {
	t.writeQueue.start()

	go func() {
		for !t.closed.Load() {
			t.read()
		}
	}()
	go func() {
		for !t.closed.Load() {
			t.write()
		}
	}()
	go func() {
		for !t.closed.Load() {
			time.Sleep(AckTimerDuration)
			t.ack()
		}
	}()
}

func (t *tunnel) stop() {
	t.closed.Store(true)
	t.connCond.Broadcast()
	t.writeQueue.stop()
}
