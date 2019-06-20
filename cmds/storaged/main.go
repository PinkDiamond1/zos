package main

import (
	"context"
	"flag"

	"github.com/rs/zerolog/log"

	"github.com/threefoldtech/zbus"
	"github.com/threefoldtech/zosv2/modules"
	"github.com/threefoldtech/zosv2/modules/storage"
)

const redisSocket = "unix:///var/run/redis.sock"
const module = "storage"

func main() {
	var (
		msgBrokerCon string
		workerNr     uint
	)

	flag.StringVar(&msgBrokerCon, "broker", redisSocket, "connection string to the message broker")
	flag.UintVar(&workerNr, "workers", 1, "number of workers")

	flag.Parse()

	storage := storage.New()
	policy := modules.StoragePolicy{Raid: modules.Single, Disks: 4, MaxPools: 0}
	if err := storage.Initialize(policy); err != nil {
		log.Fatal().Msgf("Failed to initialize storage: %s", err)
	}

	server, err := zbus.NewRedisServer(module, msgBrokerCon, workerNr)
	if err != nil {
		log.Fatal().Msgf("fail to connect to message broker server: %v", err)
	}

	server.Register(zbus.ObjectID{Name: module, Version: "0.0.1"}, storage)

	log.Info().
		Str("broker", msgBrokerCon).
		Uint("worker nr", workerNr).
		Msg("starting storaged module")

	if err := server.Run(context.Background()); err != nil {
		log.Error().Err(err).Msg("unexpected error")
	}

	log.Warn().Msgf("Exiting storaged")
}
