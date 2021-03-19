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
	"time"
)

func main() {
	// configure the zerolog for pretty commmand line feedback
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// add fields to logger
	log := log.With().Str("func", "main").Logger()

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
	log = log.With().Str("func", "main.handleConnections").Logger()

	// set up connection
	conn := proxy.NewDelayedConnection(1*time.Second, 0, "localhost:9401")

	// put logger in context
	ctx := log.WithContext(context.TODO())

	// run connection
	err := conn.Run(ctx, clientConn)
	if err != nil {
		log.Error().Err(err).Msg("connection exited with error")
	}
}
