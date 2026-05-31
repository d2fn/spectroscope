package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"

	"spectroserver/spectro"
)

// Demo wiring: capture packets on a Linux interface via AF_PACKET (no
// libpcap, no cgo) and feed each packet into Spectroscope as one
// observation tagged with iface/direction/proto/peer-port-class and
// carrying packet_size and payload_size as measures.
//
//	sudo setcap cap_net_raw,cap_net_admin=eip ./spectroserver
//	./spectroserver -iface eth0
//	# then visit http://127.0.0.1:6060/spectrogram/ui
func main() {
	ifaceFlag := flag.String("iface", "", "network interface to capture on (try `ip -br link`)")
	addr := flag.String("addr", "127.0.0.1:6060", "listen address for the UI")
	flag.Parse()

	if *ifaceFlag == "" {
		log.Fatal("must pass -iface <name>; try `ip -br link` to list interfaces")
	}

	localIPs, err := localAddrs(*ifaceFlag)
	if err != nil {
		log.Fatalf("resolve local addrs for %s: %v", *ifaceFlag, err)
	}

	ss := spectro.New(
		time.Second,
		600,
		[]string{"iface", "direction", "proto", "peer_port_class"},
		[]string{"packet_size", "payload_size"},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go ss.Start(ctx)
	go capture(ctx, ss, *ifaceFlag, localIPs)

	mux := http.NewServeMux()
	mux.Handle("/spectrogram/", http.StripPrefix("/spectrogram", ss.Handler()))

	log.Printf("spectroserver listening on http://%s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

func localAddrs(ifaceName string) (map[string]bool, error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return nil, err
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(addrs))
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		out[ipn.IP.String()] = true
	}
	return out, nil
}

func capture(ctx context.Context, ss *spectro.SpectroscopeServer, ifaceName string, localIPs map[string]bool) {
	h, err := pcapgo.NewEthernetHandle(ifaceName)
	if err != nil {
		log.Fatalf("open %s (need CAP_NET_RAW): %v", ifaceName, err)
	}
	defer h.Close()

	source := gopacket.NewPacketSource(h, layers.LayerTypeEthernet)
	source.Lazy = true
	source.NoCopy = true

	packets := source.Packets()
	for {
		select {
		case <-ctx.Done():
			return
		case pkt, ok := <-packets:
			if !ok {
				return
			}
			emitPacket(ss, ifaceName, localIPs, pkt)
		}
	}
}

func emitPacket(ss *spectro.SpectroscopeServer, ifaceName string, localIPs map[string]bool, pkt gopacket.Packet) {
	totalLen := float64(len(pkt.Data()))
	payloadLen := totalLen

	direction := "unknown"
	proto := "other"
	peerPort := 0

	if netLayer := pkt.NetworkLayer(); netLayer != nil {
		src, dst := netLayer.NetworkFlow().Endpoints()
		switch {
		case localIPs[src.String()]:
			direction = "tx"
		case localIPs[dst.String()]:
			direction = "rx"
		}
		payloadLen = float64(len(netLayer.LayerPayload()))
		switch netLayer.LayerType() {
		case layers.LayerTypeIPv4:
			proto = "ipv4"
		case layers.LayerTypeIPv6:
			proto = "ipv6"
		}
	}

	if tp := pkt.TransportLayer(); tp != nil {
		switch t := tp.(type) {
		case *layers.TCP:
			proto = "tcp"
			if direction == "tx" {
				peerPort = int(t.DstPort)
			} else {
				peerPort = int(t.SrcPort)
			}
		case *layers.UDP:
			proto = "udp"
			if direction == "tx" {
				peerPort = int(t.DstPort)
			} else {
				peerPort = int(t.SrcPort)
			}
		}
		payloadLen = float64(len(tp.LayerPayload()))
	} else if pkt.Layer(layers.LayerTypeICMPv4) != nil || pkt.Layer(layers.LayerTypeICMPv6) != nil {
		proto = "icmp"
	} else if pkt.Layer(layers.LayerTypeARP) != nil {
		proto = "arp"
	}

	ts := pkt.Metadata().Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	_ = ss.Emit(spectro.Observation{
		Time: ts,
		Dimensions: map[string]string{
			"iface":           ifaceName,
			"direction":       direction,
			"proto":           proto,
			"peer_port_class": portClass(peerPort),
		},
		Measures: map[string]float64{
			"packet_size":  totalLen,
			"payload_size": payloadLen,
		},
	})
}

func portClass(p int) string {
	switch {
	case p == 0:
		return "none"
	case p < 1024:
		return "well-known"
	case p < 49152:
		return "registered"
	default:
		return "ephemeral"
	}
}
