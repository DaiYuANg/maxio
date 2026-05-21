package config

import "testing"

func TestLoadReadsClusterTokenFromEnv(t *testing.T) {
	t.Setenv("MAXIO_CLUSTER_TOKEN", " cluster-secret ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.ClusterToken != "cluster-secret" {
		t.Fatalf("cluster token = %q, want %q", cfg.ClusterToken, "cluster-secret")
	}
}
