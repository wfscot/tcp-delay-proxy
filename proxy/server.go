package proxy

import (
	"context"
	"fmt"
	"github.com/rs/zerolog/log"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
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
	listenPort     int
	upDelay        time.Duration
	downDelay      time.Duration
	randomizeDelay bool
	upstreamAddr   string
}

func NewTcpDelayServer(listenPort int, upDelay time.Duration, downDelay time.Duration, randomizeDelay bool, upstreamAddr string) Server {
	return &tcpDelayServer{
		listenPort:     listenPort,
		upDelay:        upDelay,
		downDelay:      downDelay,
		randomizeDelay: randomizeDelay,
		upstreamAddr:   upstreamAddr,
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

	// for some reason, the listener is staying open even after the context is cancelled. force it closed.
	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	// initialize rng
	logNorm := distuv.LogNormal{
		Mu:    0,
		Sigma: 1.0,
		Src:   rand.NewSource(uint64(time.Now().Unix())),
	}

	i := 0
	for {
		i++
		log := log.With().Int("connNum", i).Logger()
		log.Debug().Msg("waiting for client connection")
		clientConn, err := ln.Accept()
		if err != nil {
			// suppress any final error messages if the context has been cancelled
			if ctx.Err() != nil {
				return nil
			}
			// otherwise, log error and continue
			log.Error().Err(err).Msg("error while accepting client connection")
			continue
		}
		log = log.With().Stringer("clientAddr", clientConn.RemoteAddr()).Logger()
		log.Info().Msg("accepted client connection")

		// put logger in context
		ctx := log.WithContext(ctx)

		// calculate up and down delays for this session
		upDelay := s.upDelay
		downDelay := s.downDelay
		if s.randomizeDelay {
			upDelay = time.Duration(uint64(logNorm.Rand() * float64(uint64(upDelay))))
			downDelay = time.Duration(uint64(logNorm.Rand() * float64(uint64(downDelay))))
		}

		// set up and run session in a routine
		go func(ctx context.Context, upDelay time.Duration, downDelay time.Duration) {
			session := NewDelayedSession(upDelay, downDelay, clientConn, s.upstreamAddr)
			err = session.Run(ctx)
			if err != nil {
				log.Error().Err(err).Msg("session exited with error")
			}
		}(ctx, upDelay, downDelay)
	}
}