package session

import "errors"

// ErrBrokerClosed means Subscribe was called after Close. The act and record
// brokers share it; the v1alpha1 event Broker that first declared it was
// deleted with the v1 stream (rpg-project#631).
var ErrBrokerClosed = errors.New("session broker is closed")
