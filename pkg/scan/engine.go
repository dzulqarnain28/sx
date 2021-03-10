//go:generate mockgen -package scan -destination=mock_engine_test.go -source=engine.go

package scan

import (
	"context"
	"net"
	"sync"

	"github.com/v-byte-cpu/sx/pkg/packet"
)

type Range struct {
	Interface *net.Interface
	Subnet    *net.IPNet
	SrcIP     net.IP
	SrcMAC    net.HardwareAddr
	StartPort uint16
	EndPort   uint16
}

type PacketSource interface {
	Packets(ctx context.Context, r *Range) <-chan *packet.BufferData
}

type Engine struct {
	src PacketSource
	rcv packet.Receiver
	snd packet.Sender
}

func NewEngine(ps PacketSource, s packet.Sender, r packet.Receiver) *Engine {
	return &Engine{src: ps, snd: s, rcv: r}
}

func (e *Engine) Start(ctx context.Context, r *Range) (<-chan interface{}, <-chan error) {
	packets := e.src.Packets(ctx, r)
	errc1 := e.rcv.ReceivePackets(ctx)
	done, errc2 := e.snd.SendPackets(ctx, packets)
	return done, mergeErrChan(ctx, errc1, errc2)
}

// generics would be helpful :)
func mergeErrChan(ctx context.Context, channels ...<-chan error) <-chan error {
	var wg sync.WaitGroup
	wg.Add(len(channels))

	out := make(chan error)
	multiplex := func(c <-chan error) {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-c:
				if !ok {
					return
				}
				select {
				case <-ctx.Done():
					return
				case out <- e:
				}
			}
		}
	}
	for _, c := range channels {
		go multiplex(c)
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}