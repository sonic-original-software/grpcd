package service

import (
	"context"
	"time"

	"git.sonicoriginal.software/grpc-protos/diagnostics"
)

// StorageCheckName is the diagnostics name the storage backend reports under
const StorageCheckName = "storage"

// StorageCheck reports the storage backend as a service dependency.
// The method value satisfies diagnostics.Check, so it is registered with the
// diagnostics service rather than being served by this package.
func (s *GRPCDServer) StorageCheck(
	ctx context.Context,
) (*diagnostics.ServiceDependency, error) {
	err := s.store.Ping(ctx)

	state := ""
	if err != nil {
		state = err.Error()
	}

	return &diagnostics.ServiceDependency{
		Address:     s.store.Address(),
		State:       state,
		Serving:     "",
		LastChecked: time.Now().Unix(),
		Details:     map[string]string{"name": s.store.Name()},
	}, err
}
