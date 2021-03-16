package main

import (
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
	bbuf := make([]byte, 1024*1024)

	// disable deadlines
	err := p.src.SetReadDeadline(time.Time{})
	if err != nil {
		log.Fatal().Err(err).Msg("error while setting read deadline")
	}
	err = p.dst.SetWriteDeadline(time.Time{})
	if err != nil {
		log.Fatal().Err(err).Msg("error while setting write deadline")
	}
	// just copy bytes in an infinite loop
	for {
		nb, err := p.src.Read(bbuf)
		if err == io.EOF {
			// this is a normal close. exit the loop.
			log.Info().Msg("connection closed by source")
			break
		} else if err != nil {
			log.Fatal().Err(err).Msg("error while reading from connection")
		}
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
				log.Fatal().Err(err).Msg("error while writing to connection")
			}

			// shouldn't happen, but just in case
			if n < 0 {
				log.Fatal().Msg("wrote negative bytes")
			} else if n == 0 {
				log.Fatal().Msg("wrote zero bytes")
			}

			// otherwise we wrote some bytes. increment the counter
			log.Debug().Int("numBytes", nb).Msg("wrote bytes")
			wc += n
		}
	}
}
