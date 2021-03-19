package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"io"
	"net"
	"sync"
	"time"
)

// represents a single delayed write. readTime is when the data was first read.
type delayedWrite struct {
	readTime time.Time
	bbuf     []byte
}

type delayedPipe struct {
	src   net.Conn
	dst   net.Conn
	delay time.Duration
}

func newDelayedPipe(src net.Conn, dst net.Conn, delay time.Duration) pipe {
	return &delayedPipe{src: src, dst: dst, delay: delay}
}

func (p *delayedPipe) Run(log zerolog.Logger) {
	log = log.With().Str("func", "delayedPipe.Run").Logger()

	// disable deadlines. note that the read routine will later overwrite the source read deadline, but that's ok.
	err := p.src.SetDeadline(time.Time{})
	if err != nil {
		log.Error().Err(err).Msg("error while disabling source connection deadline")
		return
	}
	err = p.dst.SetDeadline(time.Time{})
	if err != nil {
		log.Error().Err(err).Msg("error while disabling destination connection deadline")
		return
	}

	// run the reading and writing logic as separate routines
	// use a context both to tear down the children as well as to encapsulate the logger
	ctx, cancel := context.WithCancel(context.TODO())
	ctx = log.WithContext(ctx)

	// set up a deep buffered channel for the read routine to deliver read bytes to the write routine
	c := make(chan delayedWrite, 1024)

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		err := p.readRoutine(ctx, c)
		if err != nil {
			log.Error().Err(err).Msg("readRoutine exited with error")
		}
		cancel()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		err := p.writeRoutine(ctx, c)
		if err != nil {
			log.Error().Err(err).Msg("writeRoutine exited with error")
		}
		cancel()
		wg.Done()
	}()

	log.Info().Msg("pipe running")
	log.Debug().Msg("waiting for children to finish")
	wg.Wait()
	log.Debug().Msg("children finished. exiting.")
	log.Info().Msg("pipe shutting down")
}

// handles the read operation for the delayed pipe. only returns on error or cancelled context.
// nil return value indicates normal exit (cancelled context or normal connection close)
// non-nil return value indicates a true error
func (p *delayedPipe) readRoutine(ctx context.Context, c chan<- delayedWrite) error {
	// use the log object from the context
	log := log.Ctx(ctx).With().Str("func", "deplayedPipe.readRoutine").Logger()

	// use a static buffer of 1MB
	bbuf := make([]byte, 1024*1024)

	// receive bytes in an infinite loop
	for {
		// use a select to allow for cancelling via context
		select {
		case <-ctx.Done():
			// if the context has been cancelled, just return
			log.Debug().Msg("exiting due to cancelled context")
			return nil

		default:
			// otherwise, set a read deadline a short time in the future and attempt to read. this allows us to periodically
			// check for context cancellation
			err := p.src.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			if err != nil {
				log.Error().Err(err).Msg("error while setting source read deadline")
				return err
			}
			nb, err := p.src.Read(bbuf)
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// this is a normal read timeout due to deadline. nothing to see here.
				log.Trace().Msg("read timeout. continuing...")
				continue
			} else if err == io.EOF {
				// this is a normal close. cancel the context and return.
				log.Info().Msg("connection closed by source")
				return nil
			} else if err != nil {
				// for any other error, return it. this should result in the context getting torn down.
				log.Error().Err(err).Msg("error while reading from connection")
				return err
			}
			// otherwise we have some data
			log.Info().Int("numBytes", nb).Msg("read bytes")

			// use a go routine to delay the sending of the data to the write routine. this way the write routine is
			// dead simple. when it gets it, it sends it.
			// the one concern here is that somehow these routines execute out of order. to catch that, record the
			// current readTime and make sure that is always increasing in the write routine.
			dw := delayedWrite{
				readTime: time.Now(),
				bbuf:     make([]byte, nb),
			}
			// copy read bytes into new buffer
			// this is a little memory inefficient but that is ok for a test tool
			copy(dw.bbuf, bbuf[:nb])

			go func(dw delayedWrite) {
				// use a one-readTime timer
				t := time.NewTimer(p.delay)

				// use a select to also allow cancelling via context
				select {
				case <-ctx.Done():
					// cancel the timer and exit. discard the data.
					t.Stop()
					return

				case t := <-t.C:
					c <- dw
					log.Debug().Int("numBytes", nb).Time("readTime", dw.readTime).Time("writeTime", t).Msg("sent delayed write")
				}
			}(dw)
		}
	}
}

// handles the write operation for the delayed pipe. only returns on error or cancelled context.
// nil return value indicates normal exit (cancelled context or normal connection close)
// non-nil return value indicates a true error
func (p *delayedPipe) writeRoutine(ctx context.Context, c <-chan delayedWrite) error {
	// use the log object from the context
	log := log.Ctx(ctx).With().Str("func", "deplayedPipe.writeRoutine").Logger()

	// keep track of readTime of last write. this should always increase. this is to guard against the routines in
	// the readRoutine somehow executing out of order for back-to-back reads
	lastReadTime := time.Time{}

	for {
		// use a select statement to allow cancelling via context
		select {
		case <-ctx.Done():
			// if the context has been cancelled, just return
			log.Debug().Msg("exiting due to cancelled context")
			return nil

		case dw := <-c:
			// a delayed write is ready to write. write it now.
			log.Debug().Int("numBytes", len(dw.bbuf)).Time("readTime", dw.readTime).Time("writeTime", time.Now()).Msg("doing delayed write")

			// make sure the timestamps are always increasing
			if dw.readTime.Before(lastReadTime) {
				log.Error().Time("lastReadTime", lastReadTime).Time("dw.readTime", dw.readTime).Msg("delayed write out of order")
				return fmt.Errorf("delayed write out of order. readTime: %s, lastReadTime: %s", dw.readTime, lastReadTime)
			}
			lastReadTime = dw.readTime

			// this really should go through in one write call, but just in case, allow for partial writes and keep a write cursor
			wc := 0
			for wc < len(dw.bbuf) {
				n, err := p.dst.Write(dw.bbuf[wc:])
				if err == io.EOF {
					// this is a normal close. exit the loop.
					log.Info().Msg("connection closed by dest")
					break
				} else if err != nil {
					log.Error().Err(err).Msg("error while writing to connection")
					return err
				}

				// shouldn't happen, but just in case
				if n < 0 {
					log.Error().Msg("wrote negative bytes. aborting.")
					return errors.New("negative bytes indicated in write call")
				} else if n == 0 {
					log.Error().Msg("wrote zero bytes. aborting.")
					return errors.New("zero bytes indicated in write call")
				}

				// otherwise we wrote some bytes. increment the counter
				log.Debug().Int("numBytes", n).Msg("wrote bytes")
				wc += n
			}
		}
	}
}
