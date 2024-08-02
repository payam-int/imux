package imux

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

func createServerClient(sockets int) (clientConns []net.Conn, serverConns []net.Conn, closeChan chan interface{}) {
	closeChan = make(chan interface{})
	clientConns = make([]net.Conn, sockets)
	serverConns = make([]net.Conn, sockets)

	var ln net.Listener

	wg := &sync.WaitGroup{}

	wg.Add(2)

	go func() {
		var err error
		ln, err = net.Listen("tcp", ":9595")
		if err != nil {
			panic(err)
		}

		for i := 0; i < sockets; i++ {
			conn, err := ln.Accept()
			if err != nil {
				panic(err)
			}

			serverConns[i] = conn
		}
		wg.Done()
	}()

	go func() {
		for i := 0; i < sockets; i++ {
			conn, err := net.Dial("tcp", "localhost:9595")
			if err != nil {
				panic(err)
			}

			clientConns[i] = conn
		}
		wg.Done()
	}()

	go func() {
		<-closeChan

		for _, c := range clientConns {
			_ = c.Close()
		}
		for _, c := range serverConns {
			_ = c.Close()
		}
		_ = ln.Close()
	}()

	wg.Wait()
	return
}

func createText(size int) []byte {
	result := make([]byte, size)
	for i := 0; i < size/100; i++ {
		data := fmt.Sprintf("%099d|", i)
		for j := 0; j < 100; j++ {
			result[i*100+j] = data[j]
		}
	}

	return result
}

func TestCombineSingleSocket(t *testing.T) {
	poolSize := 1
	windowSize := 100
	chunkSize := 100
	textLen := 50000
	text := createText(textLen)
	serverCombiner, err := NewCombiner(CombinerConfig{
		Tag:        "Server",
		PoolSize:   poolSize,
		WindowSize: windowSize,
		PacketSize: chunkSize,
		AckTimeout: 20 * time.Millisecond,
	})
	assert.NoError(t, err)
	defer serverCombiner.Close()

	clientCombiner, err := NewCombiner(CombinerConfig{
		Tag:        "Client",
		PoolSize:   poolSize,
		WindowSize: windowSize,
		PacketSize: chunkSize,
		AckTimeout: 20 * time.Millisecond,
	})
	assert.NoError(t, err)
	defer clientCombiner.Close()

	clients, servers, closeChan := createServerClient(poolSize)
	defer func() {
		close(closeChan)
	}()

	for i, conn := range servers {
		serverCombiner.SetConn(i, conn)
		assert.NoError(t, err)
	}

	for i, conn := range clients {
		clientCombiner.SetConn(i, conn)
		assert.NoError(t, err)
	}

	var wg sync.WaitGroup

	wg.Add(2)

	go func() {
		_, err = clientCombiner.Write(text)
		wg.Done()
	}()

	got := make([]byte, textLen)
	go func() {
		_, err = io.ReadFull(serverCombiner, got)
		wg.Done()
	}()

	wg.Wait()

	assert.NoError(t, err)
	assert.Equal(t, text, got)
}
