package main

import (
	"context"
	"flag"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/threefoldtech/zbus"
	"github.com/threefoldtech/zosv2/modules/identity"
	"github.com/threefoldtech/zosv2/modules/provision"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	var (
		msgBrokerCon string
		resURL       string
		tnodbURL     string
		debug        bool
	)

	flag.StringVar(&msgBrokerCon, "broker", "unix:///var/run/redis.sock", "connection string to the message broker")
	flag.StringVar(&tnodbURL, "tnodb", "http://172.20.0.1:8080", "address of tenant network object database")
	flag.StringVar(&resURL, "url", "", "reservation url to poll from")
	flag.BoolVar(&debug, "debug", false, "enable debug logging")

	if debug {
		log.Logger.Level(zerolog.DebugLevel)
	}

	flag.Parse()

	nodeID, err := identity.LocalNodeID()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load node identity")
	}

	pipe, err := provision.FifoSource("/var/run/reservation.pipe")
	if err != nil {
		log.Fatal().Err(err).Msgf("failed to allocation reservation pipe")
	}

	source := pipe
	if len(resURL) != 0 {
		source = provision.CompinedSource(
			pipe,
			provision.HTTPSource(resURL, nodeID),
		)
	}

	engine := provision.New(source)

	// create context and add middlewares
	ctx := context.Background()
	client, err := zbus.NewRedisClient(msgBrokerCon)
	if err != nil {
		log.Fatal().Msgf("fail to connect to message broker server: %v", err)
	}

	ctx = provision.WithZBus(ctx, client)
	ctx = provision.WithTnoDB(ctx, tnodbURL)

	log.Info().
		Str("broker", msgBrokerCon).
		Msg("starting provision module")

	if err := engine.Run(ctx); err != nil {
		log.Error().Err(err).Msg("unexpected error")
	}
}
