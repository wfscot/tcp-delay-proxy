package main

import "github.com/rs/zerolog"

type pipe interface {
	Run(zerolog.Logger)
}
