// Package maxio exposes the embeddable MaxIO application runtime.
package maxio

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/arcgolabs/configx"
	"github.com/arcgolabs/dix"
	"github.com/lyonbrown4d/maxio/engine"
	"github.com/lyonbrown4d/maxio/index"
	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/dedupe"
	"github.com/lyonbrown4d/maxio/internal/discovery"
	"github.com/lyonbrown4d/maxio/internal/handler"
	"github.com/lyonbrown4d/maxio/internal/http"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/raft"
	"github.com/lyonbrown4d/maxio/internal/repair"
	"github.com/lyonbrown4d/maxio/internal/scheduler"
	"github.com/lyonbrown4d/maxio/internal/store"
	"github.com/lyonbrown4d/maxio/object"
	"github.com/lyonbrown4d/maxio/s3"
)

const defaultStopTimeout = 10 * time.Second

// Config is MaxIO's typed application configuration.
type Config = config.Config

// App is a built MaxIO runtime that can be embedded by another Go program.
type App struct {
	runtime     *dix.Runtime
	cfg         Config
	logger      *slog.Logger
	stopTimeout time.Duration
	started     bool
}

type buildOptions struct {
	configOptions []configx.Option
	appOptions    []dix.AppOption
	modules       []dix.Module
	stopTimeout   time.Duration
}

// Option customizes the library runtime.
type Option func(*buildOptions)

// DefaultConfig returns MaxIO's built-in configuration defaults.
func DefaultConfig() Config {
	return config.Default()
}

// LoadConfig loads typed MaxIO configuration using the same configx pipeline as the runtime.
func LoadConfig(opts ...configx.Option) (Config, error) {
	cfg, err := config.Load(opts...)
	if err != nil {
		return Config{}, fmt.Errorf("load maxio config: %w", err)
	}
	return cfg, nil
}

// WithConfigOptions appends configx options used by the config module.
func WithConfigOptions(opts ...configx.Option) Option {
	return func(options *buildOptions) {
		options.configOptions = append(options.configOptions, opts...)
	}
}

// WithDixOptions appends low-level dix app options.
func WithDixOptions(opts ...dix.AppOption) Option {
	return func(options *buildOptions) {
		options.appOptions = append(options.appOptions, opts...)
	}
}

// WithModules appends additional dix modules after MaxIO's core modules.
func WithModules(modules ...dix.Module) Option {
	return func(options *buildOptions) {
		options.modules = append(options.modules, modules...)
	}
}

// WithStopTimeout sets the graceful shutdown timeout used by Run.
func WithStopTimeout(timeout time.Duration) Option {
	return func(options *buildOptions) {
		options.stopTimeout = timeout
	}
}

// New builds a MaxIO application runtime.
func New(opts ...Option) (*App, error) {
	options := applyOptions(opts...)
	modules := defaultModules(options.configOptions...)
	modules = append(modules, options.modules...)

	appOptions := make([]dix.AppOption, 0, 1+len(options.appOptions))
	appOptions = append(appOptions,
		dix.WithModules(modules...),
	)
	appOptions = append(appOptions, options.appOptions...)

	runtime, err := dix.New("maxio", appOptions...).Build()
	if err != nil {
		return nil, fmt.Errorf("build maxio app: %w", err)
	}

	cfg := dix.MustResolveAs[config.Config](runtime.Container())
	logger := dix.MustResolveAs[*slog.Logger](runtime.Container())

	return &App{
		runtime:     runtime,
		cfg:         cfg,
		logger:      logger,
		stopTimeout: options.stopTimeout,
	}, nil
}

// Run builds, starts, waits on ctx, and gracefully stops a MaxIO runtime.
func Run(ctx context.Context, opts ...Option) error {
	app, err := New(opts...)
	if err != nil {
		return err
	}
	return app.Run(ctx)
}

// Start starts the underlying MaxIO runtime.
func (app *App) Start(ctx context.Context) error {
	if app == nil || app.runtime == nil {
		return errors.New("start maxio app: runtime is nil")
	}
	ctx = runtimeContext(ctx)

	app.logger.InfoContext(ctx, "maxio starting",
		"http_address", app.cfg.HTTPAddress,
		"data_dir", app.cfg.DataDir,
	)
	if err := app.runtime.Start(ctx); err != nil {
		return fmt.Errorf("start maxio app: %w", err)
	}
	app.started = true
	return nil
}

// Stop gracefully stops the underlying MaxIO runtime.
func (app *App) Stop(ctx context.Context) error {
	if app == nil || app.runtime == nil || !app.started {
		return nil
	}
	ctx = runtimeContext(ctx)
	if err := app.runtime.Stop(ctx); err != nil {
		return fmt.Errorf("stop maxio app: %w", err)
	}
	app.started = false
	return nil
}

// Run starts the app, blocks until ctx is done, and then stops it with a fresh stop context.
func (app *App) Run(ctx context.Context) error {
	ctx = runtimeContext(ctx)
	if err := app.Start(ctx); err != nil {
		return err
	}

	<-ctx.Done()

	stopCtx := context.WithoutCancel(ctx)
	if app.stopTimeout > 0 {
		var cancel context.CancelFunc
		stopCtx, cancel = context.WithTimeout(stopCtx, app.stopTimeout)
		defer cancel()
	}
	return app.Stop(stopCtx)
}

// Config returns the resolved runtime configuration.
func (app *App) Config() Config {
	if app == nil {
		return Config{}
	}
	return app.cfg
}

// Logger returns the resolved runtime logger.
func (app *App) Logger() *slog.Logger {
	if app == nil || app.logger == nil {
		return slog.Default()
	}
	return app.logger
}

// Objects returns the core object service for library callers.
func (app *App) Objects() (*object.Service, error) {
	if app == nil || app.runtime == nil {
		return nil, errors.New("object service unavailable: runtime is nil")
	}
	objects, err := dix.ResolveAs[*object.Service](app.runtime.Container())
	if err != nil {
		return nil, fmt.Errorf("resolve object service: %w", err)
	}
	return objects, nil
}

// Engine returns the erasure-coded storage engine for library callers.
func (app *App) Engine() (*engine.Engine, error) {
	if app == nil || app.runtime == nil {
		return nil, errors.New("engine unavailable: runtime is nil")
	}
	storageEngine, err := dix.ResolveAs[*engine.Engine](app.runtime.Container())
	if err != nil {
		return nil, fmt.Errorf("resolve engine: %w", err)
	}
	return storageEngine, nil
}

// S3 returns the S3-compatible service for library callers.
func (app *App) S3() (*s3.Service, error) {
	if app == nil || app.runtime == nil {
		return nil, errors.New("s3 service unavailable: runtime is nil")
	}
	service, err := dix.ResolveAs[*s3.Service](app.runtime.Container())
	if err != nil {
		return nil, fmt.Errorf("resolve s3 service: %w", err)
	}
	return service, nil
}

func runtimeContext(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}

// Runtime returns the underlying dix runtime for advanced integrations.
func (app *App) Runtime() *dix.Runtime {
	if app == nil {
		return nil
	}
	return app.runtime
}

// Container returns the underlying dix container for advanced integrations.
func (app *App) Container() *dix.Container {
	if app == nil || app.runtime == nil {
		return nil
	}
	return app.runtime.Container()
}

func applyOptions(opts ...Option) buildOptions {
	options := buildOptions{
		stopTimeout: defaultStopTimeout,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	return options
}

func defaultModules(configOptions ...configx.Option) []dix.Module {
	return []dix.Module{
		config.Module(configOptions...),
		raft.Module(),
		discovery.Module(),
		metadata.Module(),
		engine.Module(),
		store.Module(),
		index.Module(),
		object.Module(),
		scheduler.Module(),
		repair.Module(),
		dedupe.Module(),
		s3.Module(),
		handler.Module(),
		http.Module(),
	}
}
