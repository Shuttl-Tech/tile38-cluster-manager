package core

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/hashicorp/consul/connect"
	"inet.af/tcpproxy"
	"log"
	"math/rand"
	"net"
	"os"
	"strconv"
	"sync"
	"time"
)

// Listeners manager the leader, follower and bridge listeners
// of the sidecar.
type Listeners struct {
	bindLeader  string
	ltls        *tls.Config
	proxyLeader *tcpproxy.Proxy

	bindFollower  string
	ftls          *tls.Config
	proxyFollower *tcpproxy.Proxy

	px        *sync.RWMutex
	leader    *Peer
	followers Peers

	bindBridge string
	bridge     *tcpproxy.Proxy

	peerDialFollower peerDialer
	peerDialLeader   peerDialer
}

func (l *Listeners) SetLeader(leader *Peer) {
	l.px.Lock()
	defer l.px.Unlock()
	log.Printf("cluster leader is %s", leader.String())
	l.leader = leader
}

func (l *Listeners) SetFollowers(followers Peers) {
	l.px.Lock()
	defer l.px.Unlock()
	log.Printf("followers are %s", followers.String())
	l.followers = followers
}

func (l *Listeners) GetLeader() *Peer {
	l.px.RLock()
	defer l.px.RUnlock()
	return l.leader
}

func (l *Listeners) GetFollowers() Peers {
	l.px.RLock()
	defer l.px.RUnlock()

	return l.followers
}

func (l *Listeners) GetRandomFollower() *Peer {
	l.px.RLock()
	defer l.px.RUnlock()

	if len(l.followers) == 0 {
		return nil
	}

	return l.followers[rand.Intn(len(l.followers))]
}

type peerDialer func(context.Context, connect.Resolver) (net.Conn, error)

type ListenConfig struct {
	bindLeader       string
	bindFollower     string
	bindBridge       string
	tlsLeader        *tls.Config
	tlsFollower      *tls.Config
	peerDialFollower peerDialer
	peerDialLeader   peerDialer
}

func NewListeners(c *ListenConfig) *Listeners {
	return &Listeners{
		bindLeader:       c.bindLeader,
		ltls:             c.tlsLeader,
		bindFollower:     c.bindFollower,
		ftls:             c.tlsFollower,
		bindBridge:       c.bindBridge,
		peerDialFollower: c.peerDialFollower,
		peerDialLeader:   c.peerDialLeader,
		px:               new(sync.RWMutex),
	}
}

func (l *Listeners) Close() error {
	var errs []error
	if l.proxyLeader != nil {
		lerr := l.proxyLeader.Close()
		if lerr != nil {
			errs = append(errs, lerr)
		}
	}

	if l.proxyFollower != nil {
		ferr := l.proxyFollower.Close()
		if ferr != nil {
			errs = append(errs, ferr)
		}
	}

	if l.bridge != nil {
		berr := l.bridge.Close()
		if berr != nil {
			errs = append(errs, berr)
		}
	}

	var e string
	for _, err := range errs {
		if err == nil {
			continue
		}

		e = fmt.Sprintf("%s\n", err.Error())
	}

	if e != "" {
		return fmt.Errorf(e)
	}

	return nil
}

func (l *Listeners) Start(ctx context.Context) error {
	ll := new(tcpproxy.Proxy)
	ll.ListenFunc = fnListenTLS(ctx, l.ltls)
	ll.AddRoute(l.bindLeader, peerTarget(l.GetLeader, l.peerDialLeader))

	lf := new(tcpproxy.Proxy)
	lf.ListenFunc = fnListenTLS(ctx, l.ftls)
	lf.AddRoute(l.bindFollower, peerTarget(l.GetRandomFollower, l.peerDialFollower))

	bridge := new(tcpproxy.Proxy)
	bridge.ListenFunc = fnListenTLS(ctx, nil)
	bridge.AddRoute(l.bindBridge, peerTarget(l.GetLeader, l.peerDialLeader))

	err := bridge.Start()
	if err != nil {
		return err
	}

	err = ll.Start()
	if err != nil {
		return err
	}

	err = lf.Start()
	if err != nil {
		return err
	}

	l.proxyLeader = ll
	l.proxyFollower = lf
	l.bridge = bridge

	log.Println("listeners are online")
	return nil
}

func peerTarget(target func() *Peer, dial peerDialer) tcpproxy.Target {
	proxy := &tcpproxy.DialProxy{
		DialTimeout: time.Second,
		OnDialError: func(src net.Conn, dstDialErr error) {
			err := src.Close()
			if err != nil {
				log.Printf("error while trying to close failed connection. %s", err)
			}
		},
	}

	proxy.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		peer := target()
		if peer == nil {
			return nil, fmt.Errorf("a network peer is not available yet")
		}

		if peer.CertURI == nil {
			conn, err := new(net.Dialer).DialContext(ctx, network, fmt.Sprintf("%s:%d", peer.Address, peer.Port))
			if err != nil {
				log.Printf("ERROR DialContext: %s:%d. %s", peer.Address, peer.Port, err)
			}
			return conn, err
		}

		return dial(ctx, &connect.StaticResolver{
			Addr:    fmt.Sprintf("%s:%d", peer.Address, peer.Port),
			CertURI: peer.CertURI,
		})
	}

	return proxy
}

func fnListenTLS(ctx context.Context, c *tls.Config) func(string, string) (net.Listener, error) {
	return func(network string, addr string) (net.Listener, error) {
		lc := new(net.ListenConfig)

		l, err := lc.Listen(ctx, network, addr)
		if err != nil {
			return nil, err
		}

		if c == nil {
			return l, nil
		}

		return tls.NewListener(l, c), nil
	}
}

func freeLocalAddr() (string, int, error) {
	addr := os.Getenv("REPL_BRIDGE_IP")
	if addr == "" {
		addr = "127.0.0.1"
	}

	var port int
	p := os.Getenv("REPL_BRIDGE_PORT")
	if p != "" {
		v, err := strconv.Atoi(p)
		if err != nil {
			return "", 0, fmt.Errorf("invalid value %q for REPL_BRIDGE_PORT. %s", p, err)
		}
		port = v
	}

	l, err := net.ListenTCP("tcp", &net.TCPAddr{
		IP:   net.ParseIP(addr),
		Port: port,
	})

	if err != nil {
		return "", 0, err
	}

	defer l.Close()
	return addr, l.Addr().(*net.TCPAddr).Port, nil
}
