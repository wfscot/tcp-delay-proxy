package main

import (
	"context"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/wfscot/tcp-delay-proxy/proxy"
	"os"
	"os/signal"
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

	// establish the context with a cancel function and embed the logger
	ctx, cancel := context.WithCancel(context.Background())
	ctx = log.WithContext(ctx)

	// handle SIGINT (control+c)
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c
		log.Info().Msg("interrupt received. exiting.")
		cancel()
		// don't exit yet. let context cancellation do its magic.
	}()

	// create the server and run it
	srv := proxy.NewTcpDelayServer(9201, 0, 0, "localhost:9401")
	err := srv.Run(ctx)
	if err != nil {
		log.Error().Err(err).Msg("server exited with error")
		os.Exit(1)
	}

	os.Exit(0)
}