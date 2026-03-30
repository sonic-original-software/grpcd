package service

import (
	"context"
	"os"

	"git.sonicoriginal.software/grpc-protos/info"
)

// GetInfo returns service information
func (s *GRPCDServer) GetInfo(
	_ context.Context, _ *info.GetInfoRequest,
) (*info.GetInfoResponse, error) {
	version := os.Getenv("SERVICE_VERSION")
	serverName := os.Getenv("SERVER_NAME")

	details := map[string]string{"name": serverName}

	return &info.GetInfoResponse{
		Info: &info.ServiceInformation{
			Version: version,
			Details: details,
		},
	}, nil
}
