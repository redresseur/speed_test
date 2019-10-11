package udp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/redresseur/flogging"
	"go.uber.org/zap"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	TS_UDP_LEN = 13160
	//TS_UDP_LEN = 1024*8
)

var logger *flogging.FabricLogger

func init() {

	logging, _ := flogging.New(flogging.Config{LogSpec: "INFO",
		Format: "%{color}%{time:2006-01-02 15:04:05.000 MST}%{color} [%{module}][%{level:.4s}] " +
			"[%{shortfunc}] \"%{message}\""})

	if os.Getenv("SPEED_TEST_DEBUG") == "1" {
		logging.ActivateSpec(zap.DebugLevel.String())
	}

	logger = logging.Logger("speed_test.udp")
}

func uint32ToBytes(u uint32) []byte {
	data := make([]byte, 4)
	data[0] = byte(u & 0xff)
	data[1] = byte(u >> 8 & 0xff)
	data[2] = byte(u >> 16 & 0xff)
	data[3] = byte(u >> 24 & 0xff)
	return data
}

func bytesToUinit32(data []byte) (res uint32) {
	dataLen := len(data)
	for dataLen != 0 {
		dataLen--
		res = uint32(data[dataLen]) + res<<8
	}
	return
}

func checksum(data []byte) uint16 {
	counter := uint32(0)
	if len(data)%2 != 0 {
		data = append(data, 0)
	}

	dataLen := len(data)
	for i := 0; i < dataLen; i += 2 {
		sum := uint32(data[i]<<8) + uint32(data[i+1])
		counter += sum
		over := counter & 0xffff >> 16
		if over > 0 {
			counter += over
		}
	}

	return uint16(counter & 0xffff)
}

func Marshal(id, seq uint32, payload []byte) []byte {
	data := []byte{}
	data = append(data, uint32ToBytes(id)...)
	data = append(data, uint32ToBytes(seq)...)
	data = append(data, uint32ToBytes(uint32(len(payload)))...)
	data = append(data, payload...)
	data = append(data, uint32ToBytes(uint32(checksum(data)))...)
	return data
}

func Unmarshal(data []byte) (id, seq uint32, payload []byte, err error) {
	dataLen := len(data)
	if dataLen < 16 {
		err = errors.New("the data length is too short")
		return
	}

	//cs := bytesToUinit32(data[dataLen-4:])
	//if cs != uint32(checksum(data[:dataLen-4])) {
	//	err = errors.New("the data is broken")
	//	return
	//}

	id = bytesToUinit32(data[:4])
	seq = bytesToUinit32(data[4:8])
	payloadLen := int(bytesToUinit32(data[8:12]))
	payload = data[12 : payloadLen+12]
	return
}

func SendUdpPkt(dst string, ctx context.Context, id uint32, counter *uint64) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	udpSrv, err := net.ResolveUDPAddr("udp", dst)
	if err != nil {
		logger.Errorf("ResolveUDPAddr %s : %v", dst, err)
		return err
	}

	payload := make([]byte, TS_UDP_LEN)
	n, err := rand.Read(payload)

	udpConn, err := net.DialUDP("udp", nil, udpSrv)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			{
				logger.Debugf("the context has been canceled")
				return nil
			}
		default:
			{
				seq := atomic.AddUint64(counter, 1)
				data := Marshal(id, uint32(seq), payload[0:n])
				if wn, err := udpConn.Write(data); err != nil {
					logger.Errorf("send datagram failure: %v", err)
				}else if wn != len(data) {
					logger.Errorf("send datagram not completely: n %v wn %v", len(data), wn)
				}
			}
		}
	}
}

type StartRsp struct {
	Id   uint32 `json:"id"`
	Addr string `json:"addr"`
}

type StopRsp struct {
	Sum uint64 `json:"sum"`
}

func HttpSrv(l net.Listener, advertisingAddr string, store *sync.Map) {
	id := uint32(0)
	http.HandleFunc("/start", func(writer http.ResponseWriter, request *http.Request) {
		sid := atomic.AddUint32(&id, 1)
		writer.WriteHeader(200)
		writer.Header().Set("Content-Type", "Application/Json")
		data, _ := json.Marshal(&StartRsp{Id: sid, Addr: advertisingAddr})
		writer.Write(data)

		store.Store(sid, uint64(0))
	})

	http.HandleFunc("/stop", func(writer http.ResponseWriter, request *http.Request) {
		ssid := request.URL.Query().Get("sid")
		sid, err := strconv.Atoi(ssid)
		if err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			writer.Write([]byte(err.Error()))
		}

		// 等待未到达的包
		time.Sleep(time.Second)
		sum, ok := store.Load(uint32(sid))
		if !ok {
			writer.WriteHeader(http.StatusNotFound)
		} else {
			store.Delete(sid)
		}

		writer.WriteHeader(http.StatusOK)
		writer.Header().Set("Content-Type", "Application/Json")
		data, _ := json.Marshal(&StopRsp{Sum: sum.(uint64)})
		writer.Write(data)
	})

	http.Serve(l, nil)
}

func UdpSpeedTestStart(scheme, network_type, dst string) (*StartRsp, error) {
	transport := http.Transport{}
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return net.DialTimeout(network_type, dst, 10*time.Second)
	}

	c := http.Client{
		Transport: &transport,
	}

	// 通知服务端开始测试带宽，获取对应的id
	srsp := &StartRsp{}
	req, err := http.NewRequest("GET", "/start", nil)
	if err != nil {
		return nil, err
	}

	req.URL.Host = dst
	req.URL.Scheme = scheme

	if rsp, err := c.Do(req); err != nil {
		logger.Errorf("Request Test Start: %v", err)
		return nil, err
	} else {
		if data, err := ioutil.ReadAll(rsp.Body); err != nil {
			logger.Errorf("Read From TestStartRespond Body: %v", err)
			return nil, err
		} else {
			if err := json.Unmarshal(data, srsp); err != nil {
				logger.Errorf("The TestStartRespond is invalid: %v", err)
				return nil, err
			}
		}
	}

	return srsp, nil
}

func UdpSpeedTestStop(scheme, network_type, dst string, sid uint32) (*StopRsp, error) {
	transport := http.Transport{}
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return net.DialTimeout(network_type, dst, 10*time.Second)
	}

	c := http.Client{
		Transport: &transport,
	}

	// 获取服务端收取到的数据报结果
	strsp := &StopRsp{}
	req, err := http.NewRequest("GET", fmt.Sprintf("/stop?sid=%d", sid), nil)
	if err != nil {
		return nil, err
	}

	req.URL.Host = dst
	req.URL.Scheme = scheme

	if rsp, err := c.Do(req); err != nil {
		logger.Errorf("Request Test Stop: %v", err)
	} else {
		if data, err := ioutil.ReadAll(rsp.Body); err != nil && err != io.EOF {
			logger.Errorf("Read From TestStopRespond Body: %v", err)
			return nil, err
		} else {
			if err := json.Unmarshal(data, strsp); err != nil {
				logger.Errorf("The TestStopRespond is invalid: %v", err)
				return nil, err
			}
		}
	}

	return strsp, nil
}

func UdpSpeedTest(ctx context.Context, start func() (*StartRsp, error), stop func(sid uint32) (*StopRsp, error)) (float64, error) {
	srsp, err := start()
	if err != nil {
		return 0, err
	}

	w := sync.WaitGroup{}
	sum := uint64(0)
	for i := 0; i < runtime.NumCPU(); i++ {
		w.Add(1)
		go func() {
			SendUdpPkt(srsp.Addr, ctx, srsp.Id, &sum)
			w.Done()
		}()
	}

	w.Wait()
	logger.Debugf("Sent %d packages", sum)

	strsp, err := stop(srsp.Id)
	if err != nil {
		return 0, err
	}

	// 最終的帶寬單位為 Mbit/s, 將所有對端收到的包纍加到一起除以間隔時間，最終算出帶寬
	bandWidth := float64(strsp.Sum*TS_UDP_LEN*8) / float64(1024*1024) / 4
	logger.Debugf("Udp Server received Sum %v, Loss %v, Upload Speed %v Mbit/s",
		strsp.Sum, sum-strsp.Sum, bandWidth)
	return bandWidth, nil
}


func RecvUdpPkt(ctx context.Context, listenAddr string, store *sync.Map) error {
	ctx, _ = context.WithCancel(ctx)

	localAddr, err := net.ResolveUDPAddr("udp", listenAddr)
	if err != nil {
		logger.Errorf("ResolveUDPAddr %s Failure: %v", listenAddr, err)
		return err
	}

	udpConn, err := net.ListenUDP("udp", localAddr)
	if err != nil {
		logger.Errorf("ListenUDP %s Failure: %v", listenAddr, err)
		panic(err)
	}

	logger.Debugf("CPU Number: %v", runtime.NumCPU())
	for i:= 0; i < 4; i ++ {
		go func() {
			//runtime.LockOSThread()
			//defer runtime.UnlockOSThread()

			data := make([]byte, 2048)
			for {
				n, srcAddr, err := udpConn.ReadFromUDP(data)
				if err != nil {
					logger.Warningf("ReadFromUDP Failure: %v", err)
					continue
				}

				logger.Debugf("Recv a Package from %s : length %d", srcAddr.String(), n)
				id, seq, payload, err := Unmarshal(data[:n])
				if err != nil {
					logger.Warningf("Unmarshal Datagram Failure: %v", err)
					continue
				}

				sum := uint64(0)
				if v, ok := store.Load(id); ok {
					sum, ok = v.(uint64)
				}
				sum++
				logger.Debugf("id %d, sum %d,the package: <%d, %d>", id, sum, seq, len(payload))
				store.Store(id, sum)
			}
		}()
	}

	<-ctx.Done()
	return nil
}

//
//func RecvUdpPkt(listenAddr string, store *sync.Map) error {
//	localAddr, err := net.ResolveUDPAddr("udp", listenAddr)
//	if err != nil {
//		logger.Errorf("ResolveUDPAddr %s Failure: %v", listenAddr, err)
//		return err
//	}
//
//	udpConn, err := net.ListenUDP("udp", localAddr)
//	if err != nil {
//		logger.Errorf("ListenUDP %s Failure: %v", listenAddr, err)
//		return err
//	}
//
//	data := make([]byte, 102400)
//	for {
//		n, srcAddr, err := udpConn.ReadFromUDP(data)
//		if err != nil {
//			logger.Warningf("ReadFromUDP Failure: %v", err)
//			continue
//		}
//
//		logger.Debugf("Recv a Package from %s : length %d", srcAddr.String(), n)
//		id, seq, payload, err := Unmarshal(data[:n])
//		if err != nil {
//			logger.Warningf("Unmarshal Datagram Failure: %v", err)
//			continue
//		}
//
//		sum := uint64(0)
//		if v, ok := store.Load(id); ok {
//			sum, ok = v.(uint64)
//		}
//		sum++
//		logger.Debugf("id %d, sum %d,the package: <%d, %d>", id, sum, seq, len(payload))
//		store.Store(id, sum)
//	}
//
//	return nil
//}
