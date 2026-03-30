package service

import (
	"context"
	"time"

	"git.sonicoriginal.software/grpc-protos/diagnostics"
)

// GetDiagnostics returns diagnostic information about
// service dependencies (implements lib.DiagnosticsServiceServer)
func (s *GRPCDServer) GetDiagnostics(
	ctx context.Context, _ *diagnostics.GetDiagnosticsRequest,
) (*diagnostics.GetDiagnosticsResponse, error) {
	services := make(map[string]*diagnostics.ServiceDependency)
	err := s.store.Ping(ctx)
	storageState := ""
	if err != nil {
		storageState = err.Error()
	}

	storageStatus := &diagnostics.ServiceDependency{
		Address:     s.store.Address(),
		State:       storageState,
		Serving:     "",
		LastChecked: time.Now().Unix(),
		Details:     map[string]string{"name": s.store.Name()},
	}

	services["storage"] = storageStatus

	return &diagnostics.GetDiagnosticsResponse{Services: services}, nil
}
