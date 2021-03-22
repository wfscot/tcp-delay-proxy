package proxy

import (
	"context"
	"github.com/rs/zerolog/log"
	"net"
	"sync"
	"time"
)

// defines a generic proxy session object
// a session represents a single proxy server session created in response to a single client connection
// for now, we just have a single implementation of session so it's contained in this file.

type Session interface {
	Run(ctx context.Context, clientConn net.Conn) error
}

type session struct {
	upDelay      time.Duration
	downDelay    time.Duration
	upstreamAddr string
}

func NewDelayedSession(upDelay time.Duration, downDelay time.Duration, upStreamAddr string) Session {
	return &session{
		upDelay:      upDelay,
		downDelay:    downDelay,
		upstreamAddr: upStreamAddr,
	}
}

func (c *session) Run(ctx context.Context, clientConn net.Conn) error {
	// use the log object from the context with additional fields
	log := log.Ctx(ctx).With().Str("func", "session.Run").Logger()

	// we own the client connection. make sure it's closed.
	defer clientConn.Close()

	log.Debug().Msg("initiating session")

	// establish upstream session
	log.Debug().Msg("establishing upstream connection")
	upstreamConn, err := net.Dial("tcp", c.upstreamAddr)
	if err != nil {
		log.Error().Err(err).Str("upstreamAddr", c.upstreamAddr).Msg("error establishing upstream connection")
		return err
	}
	log = log.With().Stringer("upstreamAddr", upstreamConn.RemoteAddr()).Logger()
	log.Info().Msg("upstream connection established")
	defer upstreamConn.Close()

	// set up pipes for handling traffic in both directions. if delay is zero, use a simple pipe.
	var upPipe, downPipe Pipe
	if c.upDelay.Nanoseconds() == 0 {
		log.Debug().Msg("using simple up pipe")
		upPipe = NewSimplePipe(clientConn, upstreamConn)
	} else {
		log.Debug().Dur("upDelay", c.upDelay).Msg("using delayed up pipe")
		upPipe = NewDelayedPipe(clientConn, upstreamConn, c.upDelay)
	}
	if c.downDelay.Nanoseconds() == 0 {
		log.Debug().Msg("using simple down pipe")
		downPipe = NewSimplePipe(upstreamConn, clientConn)
	} else {
		log.Debug().Dur("downDelay", c.downDelay).Msg("using delayed down pipe")
		downPipe = NewDelayedPipe(upstreamConn, clientConn, c.downDelay)
	}
	log.Info().Msg("pipes established")

	// run the up and down pipes separately
	// use a context both to tear down the children as well as to encapsulate the logger
	ctx, cancel := context.WithCancel(ctx)
	ctx = log.WithContext(ctx)

	// remember the last error
	var lastErr error

	// run pipes in separate go routines
	// note that we don't have to explicitly handle context cancellation here as it's handled by the children
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		// enhance logger and store in context
		log := log.With().Str("direction", "up").Logger()
		ctx := log.WithContext(ctx)
		log.Debug().Msg("running up pipe")
		err := upPipe.Run(ctx)
		if err != nil {
			log.Error().Err(err).Msg("up pipe exited with error")
			lastErr = err
		}
		log.Debug().Msg("up pipe finished")
		cancel()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		// enhance logger and store in context
		log := log.With().Str("direction", "down").Logger()
		ctx := log.WithContext(ctx)
		log.Debug().Msg("running down pipe")
		err := downPipe.Run(ctx)
		if err != nil {
			log.Error().Err(err).Msg("down pipe exited with error")
			lastErr = err
		}
		log.Debug().Msg("down pipe finished")
		cancel()
		wg.Done()
	}()
	log.Info().Msg("all pipes running")

	// wait for all pipes to complete
	log.Debug().Msg("waiting for pipes to finish")
	wg.Wait()
	log.Info().Msg("all pipes finished. closing session.")

	return lastErr
}