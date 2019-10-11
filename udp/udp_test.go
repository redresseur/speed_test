package udp

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"log"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestCheckSum(t *testing.T) {
	t.Log(checksum([]byte("hello world, welcome to Russia, " +
		"hello world, welcome to Russia, hello world, welcome to Russia")))
}

func TestBytesUint32(t *testing.T) {
	data := uint32ToBytes(100086)
	assert.Equal(t, uint32(100086), bytesToUinit32(data))
}

func TestMarshal(t *testing.T) {
	payload := make([]byte, TS_UDP_LEN)
	n, err := rand.Read(payload)
	if !assert.NoError(t, err) {
		t.SkipNow()
	}

	Marshal(0, 100, payload[:n])
}

func TestUnmarshal(t *testing.T) {
	payload := make([]byte, TS_UDP_LEN)
	n, err := rand.Read(payload)
	if !assert.NoError(t, err) {
		t.SkipNow()
	}

	data := Marshal(12, 100, payload[:n])

	id, seq, data, err := Unmarshal(data)
	if assert.NoError(t, err) {
		t.SkipNow()
	}

	assert.Equal(t, 12, id)
	assert.Equal(t, 100, seq)
	assert.Equal(t, payload, data)
}

func TestSendUdpPkt(t *testing.T) {
	w := sync.WaitGroup{}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	sum := uint64(0)

	for i := 0; i < runtime.NumCPU(); i++ {
		w.Add(1)
		go func() {
			 //SendUdpPkt("192.168.0.79:8082", ctx, 1, &sum)
			//SendUdpPkt("119.23.221.167:8080", ctx, 11, &sum)
			SendUdpPkt("192.168.1.160:8081", ctx, 11, &sum)
			w.Done()
		}()
	}

	w.Wait()
	t.Logf("send %d packages", sum)
	time.Sleep(3 * time.Second)
}

func TestUdpSpeedTest(t *testing.T) {
	store := sync.Map{}
	listen_addr := "0.0.0.0:10086"
	go func() {
		l, err := net.Listen("tcp", listen_addr)
		if err != nil {
			log.Fatal(err)
		}
		HttpSrv(l, listen_addr, &store)
	}()

	// go RecvUdpPkt(listen_addr, &store)
	go RecvUdpPkt(context.Background(), listen_addr, &store)

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	bandWidth, err := UdpSpeedTest(ctx, func() (*StartRsp, error) {
		return UdpSpeedTestStart("http", "tcp", listen_addr)
	}, func(sid uint32) (*StopRsp, error) {
		return UdpSpeedTestStop("http", "tcp", listen_addr, sid)
	})
	assert.NoError(t, err )
	t.Logf("bandWidth: %v Mbit/s", bandWidth)
}

func TestUdpSpeedTest1(t *testing.T) {
	//listen_addr := "119.23.221.167:8080"
	//
	listen_addr := "192.168.1.160:8081"
	// listen_addr := "192.168.0.79:8082"
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	t.Log(UdpSpeedTest(ctx, func() (*StartRsp, error) {
		return UdpSpeedTestStart("http", "tcp", listen_addr)
	}, func(sid uint32) (*StopRsp, error) {
		return UdpSpeedTestStop("http", "tcp", listen_addr, sid)
	}))
}

func TestJson(t *testing.T)  {
	t.Log(json.Marshal(&struct {
		S string
	}{S: `{$gt: "0"}`}))
}
