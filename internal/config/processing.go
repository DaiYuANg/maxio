package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

const (
	ProcessingModeDisabled        = "disabled"
	ProcessingModeAsyncPermissive = "async_permissive"
	ProcessingModeAsyncStrict     = "async_strict"
	ProcessingModeInlineStrict    = "inline_strict"
)

func applyProcessingZeroDefaults(cfg Config) Config {
	defaults := processingDefaults()
	if cfg.ProcessingMode == "" {
		cfg.ProcessingMode = defaults.ProcessingMode
	}
	if cfg.ProcessingTimeout == "" {
		cfg.ProcessingTimeout = defaults.ProcessingTimeout
	}
	if cfg.ProcessingClamAVMode == "" {
		cfg.ProcessingClamAVMode = defaults.ProcessingClamAVMode
	}
	if cfg.ProcessingClamAVAddress == "" {
		cfg.ProcessingClamAVAddress = defaults.ProcessingClamAVAddress
	}
	if cfg.ProcessingTikaMode == "" {
		cfg.ProcessingTikaMode = defaults.ProcessingTikaMode
	}
	if cfg.ProcessingTikaURL == "" {
		cfg.ProcessingTikaURL = defaults.ProcessingTikaURL
	}
	if cfg.ProcessingTikaMaxBytes == 0 {
		cfg.ProcessingTikaMaxBytes = defaults.ProcessingTikaMaxBytes
	}
	return cfg
}

func processingDefaults() Config {
	return Config{
		ProcessingMode:          ProcessingModeAsyncPermissive,
		ProcessingTimeout:       "30s",
		ProcessingClamAVMode:    ProcessingModeInlineStrict,
		ProcessingClamAVAddress: "clamav:3310",
		ProcessingTikaMode:      ProcessingModeAsyncPermissive,
		ProcessingTikaURL:       "http://tika:9998",
		ProcessingTikaMaxBytes:  100 << 20,
	}
}

func validateProcessingConfig(cfg Config) error {
	cfg = applyProcessingZeroDefaults(cfg)
	if err := validateProcessingModes(cfg); err != nil {
		return err
	}
	if err := validateProcessingRuntime(cfg); err != nil {
		return err
	}
	if err := validateProcessingEnabledState(cfg); err != nil {
		return err
	}
	if err := validateProcessingFailOpen(cfg); err != nil {
		return err
	}
	return validateProcessingEndpoints(cfg)
}

func validateProcessingModes(cfg Config) error {
	modeChecks := map[string]string{
		"processing_mode":        cfg.ProcessingMode,
		"processing_clamav_mode": cfg.ProcessingClamAVMode,
		"processing_tika_mode":   cfg.ProcessingTikaMode,
	}
	for name, mode := range modeChecks {
		if err := validateProcessingMode(name, mode); err != nil {
			return err
		}
	}
	return nil
}

func validateProcessingRuntime(cfg Config) error {
	if _, err := parseDuration(cfg.ProcessingTimeout); err != nil {
		return fmt.Errorf("invalid config: processing_timeout: %w", err)
	}
	if cfg.ProcessingTikaMaxBytes < 0 {
		return errors.New("invalid config: processing_tika_max_bytes must be non-negative")
	}
	return nil
}

func validateProcessingEnabledState(cfg Config) error {
	mode := normalizeProcessingMode(cfg.ProcessingMode)
	if cfg.ProcessingEnabled && mode == ProcessingModeDisabled {
		return errors.New("invalid config: processing_mode cannot be disabled when processing_enabled is true")
	}
	if !cfg.ProcessingEnabled && (cfg.ProcessingClamAVEnabled || cfg.ProcessingTikaEnabled) {
		return errors.New("invalid config: processing_enabled must be true when a processing processor is enabled")
	}
	if cfg.ProcessingClamAVEnabled && normalizeProcessingMode(cfg.ProcessingClamAVMode) == ProcessingModeDisabled {
		return errors.New("invalid config: processing_clamav_mode cannot be disabled when processing_clamav_enabled is true")
	}
	if cfg.ProcessingTikaEnabled && normalizeProcessingMode(cfg.ProcessingTikaMode) == ProcessingModeDisabled {
		return errors.New("invalid config: processing_tika_mode cannot be disabled when processing_tika_enabled is true")
	}
	return nil
}

func validateProcessingFailOpen(cfg Config) error {
	if cfg.ProcessingFailOpen && hasStrictEnabledProcessingProcessor(cfg) {
		return errors.New("invalid config: processing_fail_open cannot be true when an enabled processor runs in a strict mode")
	}
	if cfg.ProcessingTikaEnabled && cfg.ProcessingTikaFailOpen && isStrictProcessingMode(cfg.ProcessingTikaMode) {
		return errors.New("invalid config: processing_tika_fail_open cannot be true when tika runs in a strict mode")
	}
	return nil
}

func validateProcessingEndpoints(cfg Config) error {
	if cfg.ProcessingClamAVEnabled {
		if err := validateProcessingTCPAddress("processing_clamav_address", cfg.ProcessingClamAVAddress); err != nil {
			return err
		}
	}
	if cfg.ProcessingTikaEnabled {
		return validateProcessingHTTPURL("processing_tika_url", cfg.ProcessingTikaURL)
	}
	return nil
}

func hasStrictEnabledProcessingProcessor(cfg Config) bool {
	return (cfg.ProcessingClamAVEnabled && isStrictProcessingMode(cfg.ProcessingClamAVMode)) ||
		(cfg.ProcessingTikaEnabled && isStrictProcessingMode(cfg.ProcessingTikaMode))
}

func isStrictProcessingMode(mode string) bool {
	switch normalizeProcessingMode(mode) {
	case ProcessingModeAsyncStrict, ProcessingModeInlineStrict:
		return true
	default:
		return false
	}
}

func validateProcessingMode(name, mode string) error {
	switch normalizeProcessingMode(mode) {
	case ProcessingModeDisabled, ProcessingModeAsyncPermissive, ProcessingModeAsyncStrict, ProcessingModeInlineStrict:
		return nil
	default:
		return fmt.Errorf("invalid config: %s must be one of %s, %s, %s, %s", name, ProcessingModeDisabled, ProcessingModeAsyncPermissive, ProcessingModeAsyncStrict, ProcessingModeInlineStrict)
	}
}

func normalizeProcessingMode(mode string) string {
	return strings.TrimSpace(strings.ToLower(mode))
}

func validateProcessingTCPAddress(name, value string) error {
	host, port, err := net.SplitHostPort(strings.TrimSpace(value))
	if err != nil || strings.TrimSpace(host) == "" || strings.TrimSpace(port) == "" {
		return fmt.Errorf("invalid config: %s must be host:port", name)
	}
	return nil
}

func validateProcessingHTTPURL(name, value string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("invalid config: %s must be an absolute HTTP URL", name)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("invalid config: %s must use http or https", name)
	}
	return nil
}

func (cfg Config) ProcessingTimeoutDuration() time.Duration {
	return parseDurationOr(cfg.ProcessingTimeout, 30*time.Second)
}
