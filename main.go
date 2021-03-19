package main

import (
	"context"
	"fmt"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/wfscot/tcp-delay-proxy/proxy"
	"net"
	"os"
	"os/signal"
	"sync"
	"time"
)

func main() {
	// configure the zerolog for pretty commmand line feedback
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// default log level is warn
	// TODO - adjust to warn
	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	// TODO - process command line args

	// establish the listener
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", 9201))
	if err != nil {
		log.Fatal().Err(err).Msg("error while establishing listener")
	}
	defer ln.Close()
	log.Info().Stringer("addr", ln.Addr()).Msg("listener established")

	// handle SIGINT (control+c)
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c
		log.Info().Msg("interrupt received. exiting.")
		ln.Close()
		os.Exit(1)
	}()

	i := 0
	for {
		i++
		conn, err := ln.Accept()
		if err != nil {
			log.Fatal().Int("connNum", i).Err(err).Msg("error while accepting connection")
		}
		sl := log.With().Int("connNum", i).Stringer("clientAddr", conn.RemoteAddr()).Logger()
		sl.Info().Msg("accepted connection")

		go handleConnection(sl, conn)
	}
}

func handleConnection(log zerolog.Logger, clientConn net.Conn) {
	defer clientConn.Close()
	log.Debug().Msg("handling connection")

	// establish upstream connection
	log.Debug().Msg("establishing upstream connection")
	upstreamConn, err := net.Dial("tcp", "localhost:9401")
	if err != nil {
		log.Error().Err(err).Msg("error establishing upstream connection")
		return
	}
	log.Info().Stringer("upstreamAddr", upstreamConn.RemoteAddr()).Msg("upstream connection established")

	// set up pipes for handling traffic in both directions
	upPipe := proxy.NewDelayedPipe(clientConn, upstreamConn, 1*time.Second)
	downPipe := proxy.NewSimplePipe(upstreamConn, clientConn)
	log.Debug().Msg("pipes established")

	ctx := context.TODO()

	// run the up and down pipes separately
	// use a context both to tear down the children as well as to encapsulate the logger
	ctx, cancel := context.WithCancel(ctx)
	ctx = log.WithContext(ctx)

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
		}
		log.Debug().Msg("down pipe finished")
		cancel()
		wg.Done()
	}()
	log.Info().Msg("all pipes running")

	// wait for all pipes to complete
	log.Debug().Msg("waiting for pipes to finish")
	wg.Wait()
	log.Info().Msg("all pipes finished. closing connection.")
}
