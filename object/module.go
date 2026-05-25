// Package object exposes MaxIO's public object service API.
package object

import (
	"context"

	"github.com/arcgolabs/dix"
)

func Module() dix.Module {
	return dix.NewModule(
		"object",
		dix.WithModuleProviders(
			dix.Provider5(NewService),
		),
		dix.Hooks(
			dix.OnStart(func(ctx context.Context, service *Service) error {
				return service.StartIndexWorker(ctx)
			}),
			dix.OnStop(func(_ context.Context, service *Service) error {
				return service.stopIndexWorker()
			}),
		),
	)
}
