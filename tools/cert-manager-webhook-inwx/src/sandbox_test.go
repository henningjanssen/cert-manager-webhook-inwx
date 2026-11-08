package main

import (
	"os"
	"testing"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/nrdcg/goinwx"
	corev1 "k8s.io/api/core/v1"
	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// TestInwxSandbox runs against the real INWX sandbox API, and is skipped unless
// INWX_SANDBOX_USERNAME, INWX_SANDBOX_PASSWORD and INWX_SANDBOX_ZONE are set.
// INWX_SANDBOX_TOTP_SECRET is needed only for accounts with 2FA.
func TestInwxSandbox(t *testing.T) {
	username, password, zone := os.Getenv("INWX_SANDBOX_USERNAME"), os.Getenv("INWX_SANDBOX_PASSWORD"), os.Getenv("INWX_SANDBOX_ZONE")
	if username == "" || password == "" || zone == "" {
		t.Skip("INWX sandbox credentials not set")
	}

	var client *realInwxClient
	s := &inwxDNSSolver{
		kubeClient: fake.NewSimpleClientset(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "inwx-credentials", Namespace: "default"},
			Data: map[string][]byte{
				"username":           []byte(username),
				"password":           []byte(password),
				defaultTOTPSecretKey: []byte(os.Getenv("INWX_SANDBOX_TOTP_SECRET")),
			},
		}),
		newClient: func(u, p string) inwxClient {
			client = &realInwxClient{c: goinwx.NewClient(u, p, &goinwx.ClientOptions{Sandbox: true})}
			return client
		},
	}
	ch := &v1alpha1.ChallengeRequest{
		ResourceNamespace: "default",
		ResolvedFQDN:      "_acme-challenge.webhook-test." + zone + ".",
		ResolvedZone:      zone + ".",
		Key:               "webhook-sandbox-test",
		Config:            &extapi.JSON{Raw: []byte(`{"secretName":"inwx-credentials"}`)},
	}
	t.Cleanup(func() {
		if err := s.CleanUp(ch); err != nil {
			t.Errorf("CleanUp: %v", err)
		}
	})

	if err := s.Present(ch); err != nil {
		t.Fatalf("Present: %v", err)
	}
	// cert-manager may repeat Present; the existing record must count as success.
	if err := s.Present(ch); err != nil {
		t.Fatalf("second Present: %v", err)
	}
}
