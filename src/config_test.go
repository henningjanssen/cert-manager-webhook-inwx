package main

import (
	"testing"

	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)


func TestLoadConfig_Nil(t *testing.T) {
	cfg, err := loadConfig(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SecretName != "" || cfg.UsernameKey != "" || cfg.PasswordKey != "" {
		t.Errorf("expected empty config for nil input, got %+v", cfg)
	}
}

func TestLoadConfig_EmptyObject(t *testing.T) {
	cfg, err := loadConfig(&extapi.JSON{Raw: []byte(`{}`)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SecretName != "" {
		t.Errorf("expected empty secretName, got %q", cfg.SecretName)
	}
}

func TestLoadConfig_FullyPopulated(t *testing.T) {
	raw := `{"secretName":"my-secret","usernameKey":"user","passwordKey":"pass"}`
	cfg, err := loadConfig(&extapi.JSON{Raw: []byte(raw)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SecretName != "my-secret" {
		t.Errorf("secretName: got %q, want %q", cfg.SecretName, "my-secret")
	}
	if cfg.UsernameKey != "user" {
		t.Errorf("usernameKey: got %q, want %q", cfg.UsernameKey, "user")
	}
	if cfg.PasswordKey != "pass" {
		t.Errorf("passwordKey: got %q, want %q", cfg.PasswordKey, "pass")
	}
}

func TestLoadConfig_OnlySecretName(t *testing.T) {
	cfg, err := loadConfig(&extapi.JSON{Raw: []byte(`{"secretName":"only-secret"}`)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SecretName != "only-secret" {
		t.Errorf("secretName: got %q, want %q", cfg.SecretName, "only-secret")
	}
	// Optional fields should remain at zero value.
	if cfg.UsernameKey != "" || cfg.PasswordKey != "" {
		t.Errorf("expected empty optional keys, got usernameKey=%q passwordKey=%q",
			cfg.UsernameKey, cfg.PasswordKey)
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	_, err := loadConfig(&extapi.JSON{Raw: []byte(`{not valid json}`)})
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestLoadConfig_MalformedType(t *testing.T) {
	// secretName is expected to be a string; an array causes a type error.
	_, err := loadConfig(&extapi.JSON{Raw: []byte(`{"secretName":123}`)})
	if err == nil {
		t.Fatal("expected error for wrong type in JSON, got nil")
	}
}
