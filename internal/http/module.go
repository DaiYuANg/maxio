// Package http wires MaxIO's HTTP server runtime.
package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	stdhttp "net/http"

	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/httpx"
	httpxfiber "github.com/arcgolabs/httpx/adapter/fiber"
	fiberapp "github.com/gofiber/fiber/v3"
	fiberadapter "github.com/gofiber/fiber/v3/middleware/adaptor"
	fiberhtml "github.com/gofiber/template/html/v3"
	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/handler"
	"github.com/lyonbrown4d/maxio/internal/http/web"
)

type httpRuntime struct {
	cfg     config.Config
	logger  *slog.Logger
	server  httpx.ServerRuntime
	app     *fiberapp.App
	service *handler.Service
}

func Module() dix.Module {
	return dix.NewModule(
		"http",
		dix.WithModuleProviders(
			dix.Provider1(newFiberApp),
			dix.Provider2(newServerRuntime),
			dix.Provider5(newHTTPRuntime),
		),
		dix.Hooks(
			dix.OnStart(startHTTPRuntime),
			dix.OnStop(stopHTTPRuntime),
		),
	)
}

func newFiberApp(cfg config.Config) *fiberapp.App {
	views := fiberhtml.NewFileSystem(web.TemplateFileSystem(), ".html")
	return fiberapp.New(fiberapp.Config{
		Views:     views,
		BodyLimit: cfg.HTTPBodyLimit,
	})
}

func newServerRuntime(logger *slog.Logger, app *fiberapp.App) httpx.ServerRuntime {
	return httpx.New(
		httpx.WithAdapter(httpxfiber.New(app)),
		httpx.WithLogger(logger),
	)
}

func newHTTPRuntime(
	cfg config.Config,
	logger *slog.Logger,
	server httpx.ServerRuntime,
	app *fiberapp.App,
	service *handler.Service,
) *httpRuntime {
	return &httpRuntime{
		cfg:     cfg,
		logger:  logger,
		server:  server,
		app:     app,
		service: service,
	}
}

func startHTTPRuntime(ctx context.Context, rt *httpRuntime) error {
	router := stdhttp.NewServeMux()
	rt.service.RegisterHTTP(router)
	nativeHandler := fiberadapter.HTTPHandler(router)
	rt.app.Get("/_admin", rt.handleAdmin)
	rt.app.All("/*", nativeHandler)

	go rt.listen(ctx)

	rt.logger.InfoContext(ctx, "http server started", "addr", rt.cfg.HTTPAddress)
	return nil
}

func (rt *httpRuntime) listen(ctx context.Context) {
	if err := rt.server.ListenAndServeContext(ctx, rt.cfg.HTTPAddress); err != nil {
		if !errors.Is(err, stdhttp.ErrServerClosed) {
			rt.logger.ErrorContext(ctx, "http server stopped", "error", err)
		}
	}
}

func stopHTTPRuntime(_ context.Context, rt *httpRuntime) error {
	rt.logger.Info("http server stopping")
	if err := rt.server.Shutdown(); err != nil {
		return fmt.Errorf("shutdown http server: %w", err)
	}
	rt.logger.Info("http server stopped")
	return nil
}

func (rt *httpRuntime) handleAdmin(c fiberapp.Ctx) error {
	c.Set("Cache-Control", "no-store")
	if err := c.Render("admin", fiberapp.Map{
		"Product": "MaxIO",
		"Title":   "MaxIO Control Plane",
	}); err != nil {
		return fmt.Errorf("render admin page: %w", err)
	}
	return nil
}
