package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/nrdcg/goinwx"
	"github.com/prometheus/client_golang/prometheus"
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
	// Login reports whether the account still needs a TOTP code via Unlock.
	Login() (needsTOTP bool, err error)
	Logout() error
	Unlock(tan string) error
	CreateRecord(req *goinwx.NameserverRecordRequest) (string, error)
	Info(req *goinwx.NameserverInfoRequest) (*goinwx.NameserverInfoResponse, error)
	DeleteRecord(id string) error
}

// realInwxClient adapts *goinwx.Client to the inwxClient interface.
type realInwxClient struct {
	c *goinwx.Client
}

func (r *realInwxClient) Login() (bool, error) {
	resp, err := r.c.Account.Login()
	if err != nil {
		return false, err
	}
	return resp.TFA == "GOOGLE-AUTH", nil
}
func (r *realInwxClient) Logout() error           { return r.c.Account.Logout() }
func (r *realInwxClient) Unlock(tan string) error { return r.c.Account.Unlock(tan) }
func (r *realInwxClient) CreateRecord(req *goinwx.NameserverRecordRequest) (string, error) {
	return r.c.Nameservers.CreateRecord(req)
}
func (r *realInwxClient) Info(req *goinwx.NameserverInfoRequest) (*goinwx.NameserverInfoResponse, error) {
	return r.c.Nameservers.Info(req)
}
func (r *realInwxClient) DeleteRecord(id string) error {
	return r.c.Nameservers.DeleteRecord(id)
}

// inwxCodeObjectExists is the INWX API result code for a record that already exists.
const inwxCodeObjectExists = 2302

// defaultTOTPSecretKey is the secret key holding the optional base32 TOTP secret.
const defaultTOTPSecretKey = "totpSecret"

// inwxDNSSolverConfig holds the configuration for the INWX DNS solver,
// deserialized from the solver's config field in the Issuer/ClusterIssuer spec.
type inwxDNSSolverConfig struct {
	// SecretName is the name of the Kubernetes secret containing INWX credentials.
	SecretName string `json:"secretName"`
	// UsernameKey is the key in the secret for the INWX username (default: "username").
	UsernameKey string `json:"usernameKey,omitempty"`
	// PasswordKey is the key in the secret for the INWX password (default: "password").
	PasswordKey string `json:"passwordKey,omitempty"`
	// TOTPSecretKey is the key in the secret for the optional base32 TOTP secret
	// of accounts with two-factor authentication (default: "totpSecret").
	TOTPSecretKey string `json:"totpSecretKey,omitempty"`
}

// inwxCredentials holds the values read from the credentials secret.
type inwxCredentials struct {
	username, password, totpSecret string
}

// inwxDNSSolver implements the cert-manager webhook Solver interface for INWX DNS.
type inwxDNSSolver struct {
	kubeClient kubernetes.Interface
	// newClient constructs an inwxClient for the given credentials.
	// Defaults to the real INWX client; may be overridden in tests.
	newClient func(username, password string) inwxClient
	// metrics holds prometheus instrumentation; nil-safe (no-ops when nil).
	metrics *solverMetrics

	totpMu          sync.Mutex
	lastTOTPCounter uint64
}

// newSolver returns an inwxDNSSolver configured to use the real INWX API.
func newSolver() *inwxDNSSolver {
	// goinwx offers no timeout and uses the default transport; without this a hung INWX call waits forever.
	http.DefaultTransport.(*http.Transport).ResponseHeaderTimeout = 30 * time.Second

	return &inwxDNSSolver{
		metrics: newSolverMetrics(prometheus.DefaultRegisterer),
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

// withClient runs fn with a logged-in INWX client, recording its span and metrics.
func (s *inwxDNSSolver) withClient(
	spanName, op string,
	ch *v1alpha1.ChallengeRequest,
	fn func(client inwxClient, zone, recordName string) error,
) (retErr error) {
	start := time.Now()
	ctx, span := otel.Tracer(instrumentationName).Start(context.Background(), spanName)
	span.SetAttributes(attribute.String("solver.name", s.Name()))
	defer func() {
		if retErr != nil {
			span.RecordError(retErr)
			span.SetStatus(codes.Error, retErr.Error())
		}
		span.End()
		s.metrics.record(op, retErr, time.Since(start))
	}()

	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return fmt.Errorf("error loading solver configuration: %w", err)
	}

	creds, err := s.getCredentials(ctx, cfg, ch.ResourceNamespace)
	if err != nil {
		return err
	}

	client := s.newClient(creds.username, creds.password)
	needsTOTP, err := client.Login()
	if err != nil {
		return fmt.Errorf("failed to authenticate with INWX: %w", err)
	}
	defer func() {
		if err := client.Logout(); err != nil {
			klog.Errorf("failed to logout from INWX: %v", err)
		}
	}()

	if needsTOTP {
		if err := s.unlock(client, creds.totpSecret); err != nil {
			return err
		}
	}

	zone, recordName := splitDNSName(ch.ResolvedFQDN, ch.ResolvedZone)
	return fn(client, zone, recordName)
}

// Present is responsible for creating the DNS TXT record to fulfil the DNS01 challenge.
func (s *inwxDNSSolver) Present(ch *v1alpha1.ChallengeRequest) error {
	return s.withClient("inwx.Present", opPresent, ch, func(client inwxClient, zone, recordName string) error {
		_, err := client.CreateRecord(&goinwx.NameserverRecordRequest{
			Domain:  zone,
			Name:    recordName,
			Type:    "TXT",
			Content: ch.Key,
			TTL:     300,
		})
		// cert-manager may repeat Present, for example after a lost response.
		var inwxErr *goinwx.ErrorResponse
		if errors.As(err, &inwxErr) && inwxErr.Code == inwxCodeObjectExists {
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to create TXT record %q in zone %q: %w", recordName, zone, err)
		}
		return nil
	})
}

// CleanUp is responsible for removing the DNS TXT record created during Present.
func (s *inwxDNSSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	return s.withClient("inwx.CleanUp", opCleanup, ch, func(client inwxClient, zone, recordName string) error {
		infoResp, err := client.Info(&goinwx.NameserverInfoRequest{
			Domain: zone,
			Type:   "TXT",
			Name:   recordName,
		})
		if err != nil {
			return fmt.Errorf("failed to list TXT records in zone %q: %w", zone, err)
		}

		for _, record := range infoResp.Records {
			if record.Content == ch.Key {
				if err := client.DeleteRecord(record.ID); err != nil {
					return fmt.Errorf("failed to delete TXT record with id %s: %w", record.ID, err)
				}
			}
		}
		return nil
	})
}

// getCredentials fetches the INWX credentials from a Kubernetes secret.
func (s *inwxDNSSolver) getCredentials(ctx context.Context, cfg inwxDNSSolverConfig, namespace string) (inwxCredentials, error) {
	if s.kubeClient == nil {
		return inwxCredentials{}, fmt.Errorf("kubernetes client is not initialised: call Initialize first")
	}
	if cfg.SecretName == "" {
		return inwxCredentials{}, fmt.Errorf("solver config must specify 'secretName'")
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

	totpSecretKey := cfg.TOTPSecretKey
	if totpSecretKey == "" {
		totpSecretKey = defaultTOTPSecretKey
	}

	secret, err := s.kubeClient.CoreV1().Secrets(namespace).Get(
		ctx,
		cfg.SecretName,
		metav1.GetOptions{},
	)
	if err != nil {
		return inwxCredentials{}, fmt.Errorf("failed to get secret %s/%s: %w", namespace, cfg.SecretName, err)
	}

	usernameBytes, ok := secret.Data[usernameKey]
	if !ok {
		return inwxCredentials{}, fmt.Errorf("key %q not found in secret %s/%s", usernameKey, namespace, cfg.SecretName)
	}
	passwordBytes, ok := secret.Data[passwordKey]
	if !ok {
		return inwxCredentials{}, fmt.Errorf("key %q not found in secret %s/%s", passwordKey, namespace, cfg.SecretName)
	}

	return inwxCredentials{
		username:   string(usernameBytes),
		password:   string(passwordBytes),
		totpSecret: string(secret.Data[totpSecretKey]),
	}, nil
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
