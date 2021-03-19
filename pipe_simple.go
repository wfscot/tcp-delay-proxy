package main

import (
	"context"
	"github.com/rs/zerolog"
	"io"
	"net"
	"time"
)

type simplePipe struct {
	src net.Conn
	dst net.Conn
}

func newSimplePipe(src net.Conn, dst net.Conn) pipe {
	return &simplePipe{src: src, dst: dst}
}

func (p *simplePipe) Run(log zerolog.Logger) {
	// TODO - get from parent
	ctx := context.TODO()

	// use the log object from the context
	//log := log.Ctx(ctx).With().Str("func", "simplePipe.Run").Logger()
	log = log.With().Str("func", "simplePipe.Run").Logger()

	// use a static buffer of 1MB
	bbuf := make([]byte, 1024*1024)

	// disable deadlines for now. note the loop below will set the source read deadline.
	err := p.src.SetDeadline(time.Time{})
	if err != nil {
		log.Error().Err(err).Msg("error while setting read deadline")
		return
	}
	err = p.dst.SetDeadline(time.Time{})
	if err != nil {
		log.Error().Err(err).Msg("error while setting write deadline")
		return
	}

	// receive bytes in an infinite loop
	for {
		// use a select to allow for cancelling via context
		select {
		case <-ctx.Done():
			// if the context has been cancelled, just return
			log.Debug().Msg("exiting due to cancelled context")
			return

		default:
			// otherwise, set a read deadline a short time in the future and attempt to read. this allows us to periodically
			// check for context cancellation
			err := p.src.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			if err != nil {
				log.Error().Err(err).Msg("error while setting source read deadline")
				return
			}
			nb, err := p.src.Read(bbuf)
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// this is a normal read timeout due to deadline. nothing to see here.
				log.Trace().Msg("read timeout. continuing...")
				continue
			} else if err == io.EOF {
				// this is a normal close. return nil.
				log.Info().Msg("connection closed by source")
				return
			} else if err != nil {
				// for any other error, return it. this should result in the context getting torn down.
				log.Error().Err(err).Msg("error while reading from connection")
				return
			}
			// otherwise we have some data. write it immediately
			log.Info().Int("numBytes", nb).Msg("read bytes")

			// this really should go through in one write call, but just in case, allow for partial writes and keep a write cursor
			wc := 0
			for wc < nb {
				n, err := p.dst.Write(bbuf[wc:nb])
				if err == io.EOF {
					// this is a normal close. exit the loop.
					log.Info().Msg("connection closed by dest")
					break
				} else if err != nil {
					log.Error().Err(err).Msg("error while writing to connection")
					return
				}

				// shouldn't happen, but just in case
				if n < 0 {
					log.Error().Msg("wrote negative bytes. aborting.")
					return
				} else if n == 0 {
					log.Error().Msg("wrote zero bytes. aborting.")
					return
				}

				// otherwise we wrote some bytes. increment the counter
				log.Debug().Int("numBytes", nb).Msg("wrote bytes")
				wc += n
			}
		}
	}
}
