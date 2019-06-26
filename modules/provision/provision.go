// Package provision is a daemon that pulls
// on reservation source, and then tries to
// apply these reservations locally.
// Note that, provision module doesn't expose
// any interface on zbus. since it should not
// be driven by users, instead all reservation
// should be pushed by the reservation source.
package provision

import (
	"context"
	"encoding/json"

	"github.com/threefoldtech/zbus"
)

// ReservationType type
type ReservationType string

const (
	// ContainerReservation type
	ContainerReservation ReservationType = "container"
)

// ReplyTo defines how report the result of the provisioning operation
type ReplyTo string

// Reservation struct
type Reservation struct {
	// ID of the reservation
	ID string `json:"id"`
	// ReplyTo is a dummy attribute to hold the 3bot address
	// we need to report to once the reservation is done
	ReplyTo ReplyTo `json:"reply-to"`
	// Type of the reservation (container, zdb, vm, etc...)
	Type ReservationType `json:"type"`
	// Data is the reservation type arguments.
	Data json.RawMessage `json:"data"`
}

// ReservationSource interface. The source
// defines how the node will get reservation requests
// then reservations are applied to the node to deploy
// a resource of the given Reservation.Type
type ReservationSource interface {
	Reservations(ctx context.Context) <-chan Reservation
}

// Engine interface
type Engine interface {
	Run(ctx context.Context) error
}

type provisioner func(client zbus.Client, reservation Reservation) error

var (
	// types defines the entry point for the different
	// reservation types. Currently only containers are
	// supported.
	types = map[ReservationType]provisioner{
		ContainerReservation: ContainerProvision,
	}
)
