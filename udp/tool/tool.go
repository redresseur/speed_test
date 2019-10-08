package main

import (
	"context"
	"flag"
	"github.com/redresseur/speed_test/udp"
	"log"
	"net"
	"os"
	"sync"
)

var (
	udp_addr         string = ""
	http_addr        string = ":7890"
	advertising_addr string = "127.0.0.1:7890"
	network_type     string = "tcp"
)

func loadFlag() {
	flag.StringVar(&udp_addr, "udp_addr", udp_addr, "the udp service listen addr")
	flag.StringVar(&http_addr, "http_addr", http_addr, "the http service listen addr")
	flag.StringVar(&advertising_addr, "advertising_addr", advertising_addr, "the advertising udp service listen addr")
	flag.StringVar(&network_type, "network_type", network_type, "the http server listen type: tcp、udp、unix")
	flag.Parse()

	if udp_addr == "" {
		udp_addr = http_addr
	}
}

func main() {
	loadFlag()
	store := sync.Map{}

	go func() {
		if network_type == "unix" {
			os.RemoveAll(http_addr)
		}

		l, err := net.Listen(network_type, http_addr)
		if err != nil {
			log.Fatal(err)
		}
		udp.HttpSrv(l, advertising_addr, &store)
	}()

	udp.RecvUdpPkt(context.Background(), udp_addr, &store)
}

