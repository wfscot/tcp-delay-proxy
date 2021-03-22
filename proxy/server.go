package proxy

import (
	"context"
	"fmt"
	"github.com/rs/zerolog/log"
	"net"
	"time"
)

// defines a generic proxy server object
// a server represents a single proxy server instance that will listen for incoming connections and proxy those to
// a specified upstream.
// for now, we just have a single server implementation so it is defined in this file.

type Server interface {
	Run(context.Context) error
}

type tcpDelayServer struct {
	listenPort      int
	upStaticDelay   time.Duration
	downStaticDelay time.Duration
	upstreamAddr    string
}

func NewTcpDelayServer(listenPort int, upStaticDelay time.Duration, downStaticDelay time.Duration, upstreamAddr string) Server {
	return &tcpDelayServer{
		listenPort:      listenPort,
		upStaticDelay:   upStaticDelay,
		downStaticDelay: downStaticDelay,
		upstreamAddr:    upstreamAddr,
	}
}

func (s *tcpDelayServer) Run(ctx context.Context) error {
	// use the log object from the context with additional fields
	log := log.Ctx(ctx).With().Str("func", "tcpDelayServer.Run").Logger()

	// use a ListenConfig so it can be torn down via context
	lc := net.ListenConfig{}

	// establish the listener on all interfaces
	ln, err := lc.Listen(ctx, "tcp", fmt.Sprintf(":%d", s.listenPort))
	if err != nil {
		log.Error().Err(err).Msg("error while establishing listener")
		return err
	}
	defer ln.Close()
	log.Info().Stringer("addr", ln.Addr()).Msg("listener established")

	i := 0
	for {
		i++
		clientConn, err := ln.Accept()
		if err != nil {
			log.Fatal().Int("connNum", i).Err(err).Msg("error while accepting client connection")
		}
		log := log.With().Int("connNum", i).Stringer("clientAddr", clientConn.RemoteAddr()).Logger()
		log.Info().Msg("accepted client connection")

		// put logger in context
		ctx := log.WithContext(ctx)

		go func(ctx context.Context) {
			// set up and run session
			session := NewDelayedSession(s.upStaticDelay, s.downStaticDelay, s.upstreamAddr)
			err = session.Run(ctx, clientConn)
			if err != nil {
				log.Error().Err(err).Msg("session exited with error")
			}
		}(ctx)
	}
}