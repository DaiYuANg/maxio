package config

import "testing"

func TestProcessingDefaults(t *testing.T) {
	cfg, err := normalize(Config{
		HTTPAddress:        ":8080",
		DataDir:            "./data",
		LogLevel:           "info",
		MetadataBackend:    "sqlite",
		CacheBackend:       "memory",
		CacheTTL:           "1m",
		PendingObjectTTL:   "1h",
		RepairInterval:     "10m",
		RepairRetryBackoff: "1s",
		DedupeInterval:     "30m",
		IndexTimeout:       "30s",
		IndexRetryBackoff:  "1s",
	})
	if err != nil {
		t.Fatalf("normalize config: %v", err)
	}
	if cfg.ProcessingMode != ProcessingModeAsyncPermissive {
		t.Fatalf("processing mode = %q, want %q", cfg.ProcessingMode, ProcessingModeAsyncPermissive)
	}
	if cfg.ProcessingTimeout != "30s" {
		t.Fatalf("processing timeout = %q, want 30s", cfg.ProcessingTimeout)
	}
	if cfg.ProcessingClamAVMode != ProcessingModeInlineStrict {
		t.Fatalf("clamav mode = %q, want %q", cfg.ProcessingClamAVMode, ProcessingModeInlineStrict)
	}
	if cfg.ProcessingClamAVAddress != "clamav:3310" {
		t.Fatalf("clamav address = %q, want clamav:3310", cfg.ProcessingClamAVAddress)
	}
	if cfg.ProcessingTikaMode != ProcessingModeAsyncPermissive {
		t.Fatalf("tika mode = %q, want %q", cfg.ProcessingTikaMode, ProcessingModeAsyncPermissive)
	}
	if cfg.ProcessingTikaURL != "http://tika:9998" {
		t.Fatalf("tika url = %q, want http://tika:9998", cfg.ProcessingTikaURL)
	}
	if cfg.ProcessingTikaMaxBytes != 100<<20 {
		t.Fatalf("tika max bytes = %d, want %d", cfg.ProcessingTikaMaxBytes, int64(100<<20))
	}
	if cfg.ProcessingTikaFailOpen {
		t.Fatal("processing_tika_fail_open default = true, want false")
	}
}

func TestProcessingRejectsDisabledModeWhenEnabled(t *testing.T) {
	cfg := minimalValidConfig()
	cfg.ProcessingEnabled = true
	cfg.ProcessingMode = ProcessingModeDisabled
	if _, err := normalize(cfg); err == nil {
		t.Fatal("expected disabled processing mode to be rejected when processing is enabled")
	}
}

func TestProcessingRejectsNegativeTikaMaxBytes(t *testing.T) {
	cfg := minimalValidConfig()
	cfg.ProcessingTikaMaxBytes = -1
	if _, err := normalize(cfg); err == nil {
		t.Fatal("expected negative processing_tika_max_bytes to be rejected")
	}
}

func minimalValidConfig() Config {
	return Config{
		HTTPAddress:        ":8080",
		DataDir:            "./data",
		LogLevel:           "info",
		MetadataBackend:    "sqlite",
		CacheBackend:       "memory",
		CacheTTL:           "1m",
		PendingObjectTTL:   "1h",
		RepairInterval:     "10m",
		RepairRetryBackoff: "1s",
		DedupeInterval:     "30m",
		IndexTimeout:       "30s",
		IndexRetryBackoff:  "1s",
		ProcessingMode:     ProcessingModeAsyncPermissive,
		ProcessingTimeout:  "30s",
	}
}
func TestProcessingRejectsDisabledModeWhenEnabledAfterNormalization(t *testing.T) {
	cfg := minimalValidConfig()
	cfg.ProcessingEnabled = true
	cfg.ProcessingMode = " Disabled "
	if _, err := normalize(cfg); err == nil {
		t.Fatal("expected normalized disabled processing mode to be rejected when processing is enabled")
	}
}

func TestProcessingRejectsInvalidTikaURLWhenEnabled(t *testing.T) {
	cfg := minimalValidConfig()
	cfg.ProcessingTikaEnabled = true
	cfg.ProcessingTikaURL = "tika:9998"
	if _, err := normalize(cfg); err == nil {
		t.Fatal("expected invalid processing_tika_url to be rejected when tika is enabled")
	}
}
func TestProcessingRejectsInvalidClamAVAddressWhenEnabled(t *testing.T) {
	cfg := minimalValidConfig()
	cfg.ProcessingClamAVEnabled = true
	cfg.ProcessingClamAVAddress = "clamav"
	if _, err := normalize(cfg); err == nil {
		t.Fatal("expected invalid processing_clamav_address to be rejected when clamav is enabled")
	}
}

func TestProcessingRejectsInvalidProcessorMode(t *testing.T) {
	cfg := Default()
	cfg.ProcessingClamAVMode = "sometimes"
	if err := Validate(cfg); err == nil {
		t.Fatal("expected invalid processing_clamav_mode to be rejected")
	}
}

func TestProcessingRejectsDisabledEnabledProcessorMode(t *testing.T) {
	cfg := Default()
	cfg.ProcessingClamAVEnabled = true
	cfg.ProcessingClamAVMode = ProcessingModeDisabled
	if err := Validate(cfg); err == nil {
		t.Fatal("expected disabled processing_clamav_mode to be rejected when clamav is enabled")
	}
}

func TestProcessingValidationAppliesZeroDefaults(t *testing.T) {
	if err := validateProcessingConfig(Config{}); err != nil {
		t.Fatalf("validate zero processing config: %v", err)
	}
}

func TestProcessingRejectsEnabledProcessorWhenPipelineDisabled(t *testing.T) {
	cfg := Default()
	cfg.ProcessingEnabled = false
	cfg.ProcessingClamAVEnabled = true
	if err := Validate(cfg); err == nil {
		t.Fatal("expected enabled processor to be rejected when processing_enabled is false")
	}
}

func TestProcessingRejectsTikaFailOpenStrictMode(t *testing.T) {
	cfg := Default()
	cfg.ProcessingEnabled = true
	cfg.ProcessingTikaEnabled = true
	cfg.ProcessingTikaMode = ProcessingModeAsyncStrict
	cfg.ProcessingTikaFailOpen = true
	if err := Validate(cfg); err == nil {
		t.Fatal("expected processing_tika_fail_open strict mode to be rejected")
	}
}

func TestProcessingRejectsClamAVStrictModeWithGlobalFailOpen(t *testing.T) {
	cfg := Default()
	cfg.ProcessingEnabled = true
	cfg.ProcessingClamAVEnabled = true
	cfg.ProcessingClamAVMode = ProcessingModeInlineStrict
	cfg.ProcessingFailOpen = true
	if err := Validate(cfg); err == nil {
		t.Fatal("expected clamav strict mode with processing_fail_open to be rejected")
	}
}

func TestProcessingRejectsTikaStrictModeWithGlobalFailOpen(t *testing.T) {
	cfg := Default()
	cfg.ProcessingEnabled = true
	cfg.ProcessingTikaEnabled = true
	cfg.ProcessingTikaMode = ProcessingModeAsyncStrict
	cfg.ProcessingFailOpen = true
	if err := Validate(cfg); err == nil {
		t.Fatal("expected tika strict mode with processing_fail_open to be rejected")
	}
}

func TestProcessingAllowsTikaFailOpenPermissiveMode(t *testing.T) {
	cfg := Default()
	cfg.ProcessingEnabled = true
	cfg.ProcessingTikaEnabled = true
	cfg.ProcessingTikaMode = ProcessingModeAsyncPermissive
	cfg.ProcessingTikaFailOpen = true
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected tika permissive fail-open config to be allowed: %v", err)
	}
}

func TestProcessingAllowsGlobalFailOpenWithoutStrictProcessors(t *testing.T) {
	cfg := Default()
	cfg.ProcessingEnabled = true
	cfg.ProcessingTikaEnabled = true
	cfg.ProcessingTikaMode = ProcessingModeAsyncPermissive
	cfg.ProcessingFailOpen = true
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected global fail-open without strict processors to be allowed: %v", err)
	}
}
