package ping

import (
	"context"
	"errors"
	"fmt"
	"github.com/redresseur/flogging"
	"github.com/redresseur/speed_test/common"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"math"
	"math/rand"
	"net"
	"os"
	"time"
)

var logger *flogging.FabricLogger

func init() {
	logger = flogging.MustGetLogger("speed_test.ping")
}

func NetworkJitter(rttTimes []float64, min, max, avg, jitter *float64) error {
	nm := float64(1000 * 1000)

	// 求标准差
	N := len(rttTimes)
	if N < 4 {
		return errors.New("the numbers of the samples cannot less than four")
	}

	// 平均值
	sum := float64(0)
	*max = rttTimes[0]
	*min = rttTimes[0]

	for i := 0; i < N; i++ {
		sum += rttTimes[i]
		if rttTimes[i] > *max {
			*max = rttTimes[i]
		}

		if rttTimes[i] < *min {
			*min = rttTimes[i]
		}
	}

	// 去掉最大值和最小值
	sum -= (*max + *min)
	*avg = sum / float64(N-2)

	// 方差
	sum = 0
	for i := 0; i < N; i++ {
		sum += (rttTimes[i] - *avg) * (rttTimes[i] - *avg)
	}

	// 去掉最大值和最小值
	//sum -= ((*max-*avg)*(*max-*avg) + (*min-*avg)*(*min-*avg))
	//*jitter = sum / float64(N-3)
	*jitter = sum / float64(N-1)
	*jitter = math.Sqrt(*jitter)

	*min /= nm
	*max /= nm
	*avg /= nm
	*jitter /= nm

	return nil
}

func ipv4ICMPReply(msg *icmp.Message) (e *icmp.Echo, err error) {
	var (
		ok bool
	)

	switch msg.Type.(ipv4.ICMPType) {
	case ipv4.ICMPTypeEchoReply:
		{
			if msg.Code != 0 {
				return nil, errors.New("the code is invalid")
			}

			if e, ok = msg.Body.(*icmp.Echo); !ok {
				return nil, errors.New("the body is invalid")
			}
		}
	case ipv4.ICMPTypeDestinationUnreachable:
		{
			return nil, errors.New("the destination is unreachable")
		}
	case ipv4.ICMPTypeTimestampReply:
	case ipv4.ICMPTypeRouterSolicitation:
	case ipv4.ICMPTypeTimeExceeded:
		{
			return nil, errors.New("ttl exceeded")
		}
	default:
		return nil, fmt.Errorf("the type %v is not supported", msg.Type.(ipv4.ICMPType))
	}

	return
}

func ping(pc *icmp.PacketConn, dst net.Addr, seq int, payload []byte) <-chan interface{} {
	reply := make(chan interface{}, 1)

	go func() {
		defer close(reply)
		wm := icmp.Message{
			Type: ipv4.ICMPTypeEcho, Code: 0,
			Body: &icmp.Echo{
				ID: os.Getpid() & 0xffff, Seq: int(seq),
				Data: payload,
			},
		}

		wb, err := wm.Marshal(nil)
		if _, err = pc.WriteTo(wb, dst); err != nil {
			reply <- err
			return
		}

		rb := make([]byte, 1500)
		n, _, err := pc.ReadFrom(rb)
		if err != nil {
			reply <- err
			return
		}

		res, err := icmp.ParseMessage(ipv4.ICMPType.Protocol(0), rb[:n])
		if err != nil {
			reply <- err
			return
		}

		reply <- res
		return
	}()

	return reply
}

func Ping(dst string, ctx context.Context) (res []float64, loss int, err error) {
	payload := make([]byte, 64)

	n, _ := rand.Read(payload)
	payload = payload[:n]

	ts := map[int]time.Time{}

	pc, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return nil, 0, err
	}

	dstAddr, err := net.ResolveIPAddr("ip4:icmp", dst)
	if err != nil {
		return nil, 0, err
	}

	for seq := 0; ; seq++ {
		ts[seq] = time.Now()
		pc.SetDeadline(time.Now().Add(2 * time.Second))
		replay := ping(pc, dstAddr, seq, payload)
		select {
		case <-ctx.Done():
			{
				return
			}
		case r := <-replay:
			if err, ok := r.(error); ok {
				logger.Errorf("ipv4, ping %s %d: %v ", dst, seq, err)
				loss++
				continue
			}

			if e, err := ipv4ICMPReply(r.(*icmp.Message)); err != nil {
				logger.Errorf("ipv4, echo reply failure: %v", err)
			} else if e != nil {
				if t, ok := ts[e.Seq]; ok {
					res = append(res, float64(time.Since(t).Nanoseconds()))
					delete(ts, e.Seq)
				} else {
					logger.Errorf("ipv4, loss the package: %v", e.Seq)
				}
			}
		}
	}

	return
}

func NewWorkDelayTest(dst string, ctx context.Context) (common.Rtt, error) {
	rtt := common.Rtt{}
	rttTimes, loss, err := Ping(dst, ctx)
	if err != nil {
		return rtt, err
	}

	sum := len(rttTimes) + loss
	if sum == 0 {
		return rtt, errors.New("send none packages")
	}

	rtt.Loss = float64(loss) / float64(sum)
	logger.Debugf("Ping Sum Number: %v, loss rate: %v", sum, loss)

	if 1.0 == rtt.Loss {
		return rtt, nil
	}

	if len(rttTimes) < 4 {
		for i := 0; i < len(rttTimes); i++ {
			rtt.Avg += rttTimes[i]
		}

		rtt.Avg /= float64(len(rttTimes))
		rtt.Avg /= (1000 * 1000)
	} else {
		if err := NetworkJitter(rttTimes, &rtt.Min, &rtt.Max, &rtt.Avg, &rtt.Jitter); err != nil {
			return rtt, err
		}
	}

	logger.Debugf("min %v max %v avgTime %v ms jitter %v ms", rtt.Min, rtt.Max, rtt.Avg, rtt.Jitter)
	return rtt, nil
}
