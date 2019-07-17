package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/threefoldtech/zbus"
	"github.com/threefoldtech/zosv2/modules"
	"github.com/threefoldtech/zosv2/modules/identity"

	"github.com/cenkalti/backoff"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/threefoldtech/zosv2/modules/network"
	"github.com/threefoldtech/zosv2/modules/network/ifaceutil"
	"github.com/threefoldtech/zosv2/modules/network/tnodb"
	"github.com/threefoldtech/zosv2/modules/zinit"
)

const redisSocket = "unix:///var/run/redis.sock"
const module = "network"

func main() {
	var (
		tnodbURL string
		root     string
		broker   string
	)

	flag.StringVar(&root, "root", "/var/cache/modules/network", "root path of the module")
	flag.StringVar(&broker, "broker", redisSocket, "connection string to broker")
	flag.StringVar(&tnodbURL, "tnodb", "https://tnodb.dev.grid.tf", "address of tenant network object database")

	flag.Parse()

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	if err := os.MkdirAll(root, 0750); err != nil {
		log.Error().Err(err).Msgf("fail to create module root")
	}

	db := tnodb.NewHTTPHTTPTNoDB(tnodbURL)

	if err := ifaceutil.SetLoUp(); err != nil {
		return
	}

	if err := bootstrap(); err != nil {
		log.Error().Err(err).Msg("failed to bootstrap network")
		os.Exit(1)
	}

	log.Info().Msg("network bootstraped successfully")
	var (
		nodeID identity.Identifier
		err    error
	)
	nodeID, err = identity.LocalNodeID()
	for err != nil {
		log.Info().Msg("wait for node identity to be generated")
		time.Sleep(time.Second * 1)
		nodeID, err = identity.LocalNodeID()
	}

	networker := network.NewNetworker(nodeID, db, root)

	if err := publishIfaces(db); err != nil {
		log.Error().Err(err).Msg("failed to publish network interfaces to tnodb")
		os.Exit(1)
	}

	ifaceVersion := -1
	exitIface, err := db.ReadPubIface(nodeID)
	if err == nil {
		if err := configuePubIface(exitIface); err != nil {
			log.Error().Err(err).Msg("failed to configure public interface")
			os.Exit(1)
		}
		ifaceVersion = exitIface.Version
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chIface := watchPubIface(ctx, nodeID, db, ifaceVersion)
	go func(ctx context.Context, ch <-chan *network.PubIface) {
		for {
			select {
			case iface := <-ch:
				_ = configuePubIface(iface)
			case <-ctx.Done():
				return
			}
		}
	}(ctx, chIface)

	// watcher := network.NewWatcher(nodeID, db)
	// chNetID := watcher.Watch(ctx)
	// go networkConfigWorker(ctx, chNetID, networker)

	// time.Sleep(time.Second * 5)

	if err := startServer(ctx, broker, networker); err != nil {
		log.Error().Err(err).Msg("fail to start networkd")
	}
}

func bootstrap() error {
	f := func() error {

		z := zinit.New("")
		if err := z.Connect(); err != nil {
			return err
		}

		log.Info().Msg("Start network bootstrap")
		if err := network.Bootstrap(); err != nil {
			return err
		}

		log.Info().Msg("writing udhcp init service")

		err := zinit.AddService("dhcp_zos", zinit.InitService{
			Exec:    fmt.Sprintf("/sbin/udhcpc -v -f -i %s -s /usr/share/udhcp/simple.script", network.DefaultBridge),
			Oneshot: false,
			After:   []string{},
		})
		if err != nil {
			return err
		}

		return z.Monitor("dhcp_zos")
	}

	errHandler := func(err error, t time.Duration) {
		if err != nil {
			log.Error().Err(err).Msg("error while trying to bootstrap network")
		}
	}

	return backoff.RetryNotify(f, backoff.NewExponentialBackOff(), errHandler)
}

func publishIfaces(db network.TNoDB) error {
	f := func() error {
		log.Info().Msg("try to publish interfaces to TNoDB")
		return db.PublishInterfaces()
	}
	errHandler := func(err error, _ time.Duration) {
		if err != nil {
			log.Error().Err(err).Msg("error while trying to publish the node interaces")
		}
	}

	return backoff.RetryNotify(f, backoff.NewExponentialBackOff(), errHandler)
}

func startServer(ctx context.Context, broker string, networker modules.Networker) error {

	server, err := zbus.NewRedisServer(module, broker, 1)
	if err != nil {
		log.Error().Err(err).Msgf("fail to connect to message broker server")
	}

	server.Register(zbus.ObjectID{Name: module, Version: "0.0.1"}, networker)

	log.Info().
		Str("broker", broker).
		Uint("worker nr", 1).
		Msg("starting networkd module")

	return server.Run(context.Background())
}

func networkConfigWorker(ctx context.Context, c <-chan modules.NetID, networker modules.Networker) {
	var (
		netID modules.NetID
		nw    modules.Network
		err   error
	)

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("network config worker done")
			return

		case netID = <-c:
			nw, err = networker.GetNetwork(netID)
			if err != nil {
				log.Error().
					Str("network ID", string(netID)).
					Err(err).
					Msg("failed to read network")
				continue
			}
		}

		log.Info().
			Str("network ID", string(netID)).
			Msg("configuring network")

		if err := networker.ApplyNetResource(nw); err != nil {
			log.Error().
				Str("network ID", string(netID)).
				Err(err).
				Msg("failed to configure network")
		}
	}
}
