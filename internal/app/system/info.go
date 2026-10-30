package system

import (
	"context"
	"fmt"

	"github.com/magpeek/mivo/internal/domain/system"
)

// Info describes the server: product name, build values, API version and the persisted
// server ID. The identity is ensured, so Info works even before EnsureIdentity ran.
func (service *Service) Info(ctx context.Context) (system.Info, error) {
	identity, err := service.EnsureIdentity(ctx)
	if err != nil {
		return system.Info{}, fmt.Errorf("system info: %w", err)
	}

	return system.Info{
		ProductName: service.productName,
		Version:     service.build.Version,
		Commit:      service.build.Commit,
		BuildTime:   service.build.BuildTime,
		ServerID:    identity.ID,
		APIVersion:  system.APIVersion,
	}, nil
}
