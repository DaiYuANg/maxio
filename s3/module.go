// Package s3 provides MaxIO's S3-compatible HTTP endpoint.
package s3

import (
	"log/slog"

	"github.com/arcgolabs/dix"
	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/object"
)

func Module() dix.Module {
	return dix.NewModule(
		"s3",
		dix.WithModuleProviders(
			dix.Provider3(newServiceFromRuntimeConfig),
			dix.Provider1(NewEndpoint),
		),
	)
}

func newServiceFromRuntimeConfig(objects *object.Service, logger *slog.Logger, cfg config.Config) *Service {
	return NewService(objects, logger, Config{
		DataDir:    cfg.DataDir,
		PathPrefix: "/s3",
		AccessKey:  cfg.S3AccessKey,
		SecretKey:  cfg.S3SecretKey,
		Region:     cfg.S3Region,
	})
}
