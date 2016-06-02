package main

import (
	"io"
	"net"
	"os"
	"sync"

	"github.com/subgraph/fw-daemon/socks5"
)

type socksChainConfig struct {
	TargetSocksNet  string
	TargetSocksAddr string
	ListenSocksNet  string
	ListenSocksAddr string
}

type socksChain struct {
	cfg      *socksChainConfig
	dbus     *dbusServer
	listener net.Listener
	wg       *sync.WaitGroup
}

type socksChainSession struct {
	cfg          *socksChainConfig
	clientConn   net.Conn
	upstreamConn net.Conn
	req          *socks5.Request
	bndAddr      *socks5.Address
	optData      []byte
}

func NewSocksChain(cfg *socksChainConfig, wg *sync.WaitGroup, dbus *dbusServer) *socksChain {
	chain := socksChain{
		cfg:  cfg,
		dbus: dbus,
		wg:   wg,
	}
	return &chain
}

// Start initializes the SOCKS 5 server and starts
// accepting connections.
func (s *socksChain) start() {
	var err error
	s.listener, err = net.Listen(s.cfg.ListenSocksNet, s.cfg.ListenSocksAddr)
	if err != nil {
		log.Error("ERR/socks: Failed to listen on the socks address: %v", err)
		os.Exit(1)
	}

	s.wg.Add(1)
	go s.socksAcceptLoop()
}

func (s *socksChain) socksAcceptLoop() error {
	defer s.wg.Done()
	defer s.listener.Close()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if e, ok := err.(net.Error); ok && !e.Temporary() {
				log.Info("ERR/socks: Failed to Accept(): %v", err)
				return err
			}
			continue
		}
		session := &socksChainSession{cfg: s.cfg, clientConn: conn}
		go session.sessionWorker()
	}
}

func (c *socksChainSession) sessionWorker() {
	defer c.clientConn.Close()

	clientAddr := c.clientConn.RemoteAddr()
	log.Info("INFO/socks: New connection from: %v", clientAddr)

	// Do the SOCKS handshake with the client, and read the command.
	var err error
	if c.req, err = socks5.Handshake(c.clientConn); err != nil {
		log.Info("ERR/socks: Failed SOCKS5 handshake: %v", err)
		return
	}

	pinfo := c.findProcessForConnection(c.clientConn)
	if pinfo == nil {
		log.Warning("No proc found for %s", printPacket(pkt, fw.dns.Lookup(pkt.Dst)))
		pkt.Accept()
		return
	}

	// target address of the socks connection
	addr := c.req.Addr.String()

	switch c.req.Cmd {
	case socks5.CommandTorResolve, socks5.CommandTorResolvePTR:
		err = c.dispatchTorSOCKS()

		// If we reach here, the request has been dispatched and completed.
		if err == nil {
			// Successfully even, send the response back with the addresc.
			c.req.ReplyAddr(socks5.ReplySucceeded, c.bndAddr)
		}
		return
	case socks5.CommandConnect:
	default:
		// Should *NEVER* happen, validated as part of handshake.
		log.Info("BUG/socks: Unsupported SOCKS command: 0x%02x", c.req.Cmd)
		c.req.Reply(socks5.ReplyCommandNotSupported)
		return
	}

	err = c.dispatchTorSOCKS()
	if err != nil {
		return
	}
	c.req.Reply(socks5.ReplySucceeded)
	defer c.upstreamConn.Close()

	if c.optData != nil {
		if _, err = c.upstreamConn.Write(c.optData); err != nil {
			log.Info("ERR/socks: Failed writing OptData: %v", err)
			return
		}
		c.optData = nil
	}

	// A upstream connection has been established, push data back and forth
	// till the session is done.
	var wg sync.WaitGroup
	wg.Add(2)

	copyLoop := func(dst, src net.Conn) {
		defer wg.Done()
		defer dst.Close()

		io.Copy(dst, src)
	}
	go copyLoop(c.upstreamConn, c.clientConn)
	go copyLoop(c.clientConn, c.upstreamConn)

	wg.Wait()
	log.Info("INFO/socks: Closed SOCKS connection from: %v", clientAddr)
}

func (c *socksChainSession) dispatchTorSOCKS() (err error) {
	c.upstreamConn, c.bndAddr, err = socks5.Redispatch(c.cfg.TargetSocksNet, c.cfg.TargetSocksAddr, c.req)
	if err != nil {
		c.req.Reply(socks5.ErrorToReplyCode(err))
	}
	return
}
