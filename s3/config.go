package s3

import (
	"log/slog"
	"strings"
)

// Config controls the standalone S3 compatibility layer.
type Config struct {
	DataDir    string
	PathPrefix string
	AccessKey  string
	SecretKey  string
	Region     string
}

type Option func(*serviceOptions)

type serviceOptions struct {
	logger *slog.Logger
	cfg    Config
}

func New(objects ObjectStore, opts ...Option) *Service {
	options := serviceOptions{
		logger: slog.Default(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	if options.logger == nil {
		options.logger = slog.Default()
	}
	cfg := options.cfg.normalized()
	return &Service{
		objects:   objects,
		logger:    options.logger,
		cfg:       cfg,
		multipart: newMultipartStore(cfg),
	}
}

func NewService(objects ObjectStore, logger *slog.Logger, cfg Config) *Service {
	return New(objects, WithLogger(logger), WithConfig(cfg))
}

func WithConfig(cfg Config) Option {
	return func(options *serviceOptions) {
		options.cfg = cfg
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(options *serviceOptions) {
		if logger != nil {
			options.logger = logger
		}
	}
}

func WithPathPrefix(prefix string) Option {
	return func(options *serviceOptions) {
		options.cfg.PathPrefix = prefix
	}
}

func (cfg Config) normalized() Config {
	cfg.DataDir = strings.TrimSpace(cfg.DataDir)
	if cfg.DataDir == "" {
		cfg.DataDir = "./data"
	}
	cfg.PathPrefix = normalizePathPrefix(cfg.PathPrefix)
	if cfg.PathPrefix == "" {
		cfg.PathPrefix = defaultPathPrefix
	}
	cfg.AccessKey = strings.TrimSpace(cfg.AccessKey)
	cfg.SecretKey = strings.TrimSpace(cfg.SecretKey)
	cfg.Region = strings.TrimSpace(cfg.Region)
	return cfg
}

func (s *Service) PathPrefix() string {
	if s == nil {
		return defaultPathPrefix
	}
	return s.cfg.PathPrefix
}
