package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/nrdcg/goinwx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

// inwxClient is the subset of the goinwx API needed by this solver.
// Defining it as an interface enables injection of test doubles.
type inwxClient interface {
	Login() error
	Logout() error
	CreateRecord(req *goinwx.NameserverRecordRequest) (string, error)
	Info(req *goinwx.NameserverInfoRequest) (*goinwx.NameserverInfoResponse, error)
	DeleteRecord(id string) error
}

// realInwxClient adapts *goinwx.Client to the inwxClient interface.
type realInwxClient struct {
	c *goinwx.Client
}

func (r *realInwxClient) Login() error  { _, err := r.c.Account.Login(); return err }
func (r *realInwxClient) Logout() error { return r.c.Account.Logout() }
func (r *realInwxClient) CreateRecord(req *goinwx.NameserverRecordRequest) (string, error) {
	return r.c.Nameservers.CreateRecord(req)
}
func (r *realInwxClient) Info(req *goinwx.NameserverInfoRequest) (*goinwx.NameserverInfoResponse, error) {
	return r.c.Nameservers.Info(req)
}
func (r *realInwxClient) DeleteRecord(id string) error {
	return r.c.Nameservers.DeleteRecord(id)
}

// inwxDNSSolverConfig holds the configuration for the INWX DNS solver,
// deserialized from the solver's config field in the Issuer/ClusterIssuer spec.
type inwxDNSSolverConfig struct {
	// SecretName is the name of the Kubernetes secret containing INWX credentials.
	SecretName string `json:"secretName"`
	// UsernameKey is the key in the secret for the INWX username (default: "username").
	UsernameKey string `json:"usernameKey,omitempty"`
	// PasswordKey is the key in the secret for the INWX password (default: "password").
	PasswordKey string `json:"passwordKey,omitempty"`
}

// inwxDNSSolver implements the cert-manager webhook Solver interface for INWX DNS.
type inwxDNSSolver struct {
	kubeClient kubernetes.Interface
	// newClient constructs an inwxClient for the given credentials.
	// Defaults to the real INWX client; may be overridden in tests.
	newClient func(username, password string) inwxClient
	// metrics holds prometheus instrumentation; nil-safe (no-ops when nil).
	metrics *solverMetrics
}

// newSolver returns an inwxDNSSolver configured to use the real INWX API.
func newSolver() *inwxDNSSolver {
	return &inwxDNSSolver{
		metrics: newSolverMetrics(defaultRegisterer()),
		newClient: func(username, password string) inwxClient {
			return &realInwxClient{
				c: goinwx.NewClient(username, password, &goinwx.ClientOptions{Sandbox: false}),
			}
		},
	}
}

// Name returns the solver name used to identify this solver in the cert-manager config.
func (s *inwxDNSSolver) Name() string { return "inwx" }

// Initialize is called once when the webhook starts and receives a Kubernetes
// client config for interacting with the cluster.
func (s *inwxDNSSolver) Initialize(kubeClientConfig *rest.Config, _ <-chan struct{}) error {
	cl, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}
	s.kubeClient = cl
	return nil
}

// Present is responsible for creating the DNS TXT record to fulfil the DNS01 challenge.
func (s *inwxDNSSolver) Present(ch *v1alpha1.ChallengeRequest) (retErr error) {
	start := time.Now()
	ctx, span := otel.Tracer(instrumentationName).Start(context.Background(), "inwx.Present")
	span.SetAttributes(attribute.String("solver.name", s.Name()))
	defer func() {
		if retErr != nil {
			span.RecordError(retErr)
			span.SetStatus(codes.Error, retErr.Error())
		}
		span.End()
		s.metrics.record(opPresent, retErr, time.Since(start))
	}()

	cfg, err := loadConfig(ch.Config)
	if err != nil {
		retErr = fmt.Errorf("error loading solver configuration: %w", err)
		return
	}

	username, password, err := s.getCredentials(ctx, cfg, ch.ResourceNamespace)
	if err != nil {
		retErr = err
		return
	}

	zone, recordName := splitDNSName(ch.ResolvedFQDN, ch.ResolvedZone)

	client := s.newClient(username, password)
	if err := client.Login(); err != nil {
		retErr = fmt.Errorf("failed to authenticate with INWX: %w", err)
		return
	}
	defer func() {
		if err := client.Logout(); err != nil {
			klog.Errorf("failed to logout from INWX: %v", err)
		}
	}()

	if _, err = client.CreateRecord(&goinwx.NameserverRecordRequest{
		Domain:  zone,
		Name:    recordName,
		Type:    "TXT",
		Content: ch.Key,
		TTL:     300,
	}); err != nil {
		retErr = fmt.Errorf("failed to create TXT record %q in zone %q: %w", recordName, zone, err)
		return
	}

	return nil
}

// CleanUp is responsible for removing the DNS TXT record created during Present.
func (s *inwxDNSSolver) CleanUp(ch *v1alpha1.ChallengeRequest) (retErr error) {
	start := time.Now()
	ctx, span := otel.Tracer(instrumentationName).Start(context.Background(), "inwx.CleanUp")
	span.SetAttributes(attribute.String("solver.name", s.Name()))
	defer func() {
		if retErr != nil {
			span.RecordError(retErr)
			span.SetStatus(codes.Error, retErr.Error())
		}
		span.End()
		s.metrics.record(opCleanup, retErr, time.Since(start))
	}()

	cfg, err := loadConfig(ch.Config)
	if err != nil {
		retErr = fmt.Errorf("error loading solver configuration: %w", err)
		return
	}

	username, password, err := s.getCredentials(ctx, cfg, ch.ResourceNamespace)
	if err != nil {
		retErr = err
		return
	}

	zone, recordName := splitDNSName(ch.ResolvedFQDN, ch.ResolvedZone)

	client := s.newClient(username, password)
	if err := client.Login(); err != nil {
		retErr = fmt.Errorf("failed to authenticate with INWX: %w", err)
		return
	}
	defer func() {
		if err := client.Logout(); err != nil {
			klog.Errorf("failed to logout from INWX: %v", err)
		}
	}()

	infoResp, err := client.Info(&goinwx.NameserverInfoRequest{
		Domain: zone,
		Type:   "TXT",
		Name:   recordName,
	})
	if err != nil {
		retErr = fmt.Errorf("failed to list TXT records in zone %q: %w", zone, err)
		return
	}

	for _, record := range infoResp.Records {
		if record.Content == ch.Key {
			if err := client.DeleteRecord(record.ID); err != nil {
				retErr = fmt.Errorf("failed to delete TXT record with id %s: %w", record.ID, err)
				return
			}
		}
	}

	return nil
}

// getCredentials fetches the INWX username and password from a Kubernetes secret.
func (s *inwxDNSSolver) getCredentials(ctx context.Context, cfg inwxDNSSolverConfig, namespace string) (string, string, error) {
	if s.kubeClient == nil {
		return "", "", fmt.Errorf("kubernetes client is not initialised: call Initialize first")
	}
	if cfg.SecretName == "" {
		return "", "", fmt.Errorf("solver config must specify 'secretName'")
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	usernameKey := cfg.UsernameKey
	if usernameKey == "" {
		usernameKey = "username"
	}
	passwordKey := cfg.PasswordKey
	if passwordKey == "" {
		passwordKey = "password"
	}

	secret, err := s.kubeClient.CoreV1().Secrets(namespace).Get(
		ctx,
		cfg.SecretName,
		metav1.GetOptions{},
	)
	if err != nil {
		return "", "", fmt.Errorf("failed to get secret %s/%s: %w", namespace, cfg.SecretName, err)
	}

	usernameBytes, ok := secret.Data[usernameKey]
	if !ok {
		return "", "", fmt.Errorf("key %q not found in secret %s/%s", usernameKey, namespace, cfg.SecretName)
	}
	passwordBytes, ok := secret.Data[passwordKey]
	if !ok {
		return "", "", fmt.Errorf("key %q not found in secret %s/%s", passwordKey, namespace, cfg.SecretName)
	}

	return string(usernameBytes), string(passwordBytes), nil
}

// splitDNSName splits a fully-qualified DNS name into zone and relative record name.
// Both fqdn and zone may include or omit the trailing dot.
func splitDNSName(fqdn, zone string) (string, string) {
	fqdn = strings.TrimSuffix(fqdn, ".")
	zone = strings.TrimSuffix(zone, ".")
	recordName := strings.TrimSuffix(fqdn, "."+zone)
	if recordName == fqdn {
		// FQDN and zone are identical — record is at the zone apex.
		recordName = "@"
	}
	return zone, recordName
}

// loadConfig deserialises the raw JSON solver config into a typed struct.
func loadConfig(cfgJSON *extapi.JSON) (inwxDNSSolverConfig, error) {
	cfg := inwxDNSSolverConfig{}
	if cfgJSON == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
		return cfg, fmt.Errorf("error decoding solver config: %w", err)
	}
	return cfg, nil
}
