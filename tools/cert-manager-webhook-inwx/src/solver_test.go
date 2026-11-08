package main

import (
	"errors"
	"testing"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/nrdcg/goinwx"
	corev1 "k8s.io/api/core/v1"
	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

// ---------------------------------------------------------------------------
// Test double
// ---------------------------------------------------------------------------

// fakeInwxClient is a controllable test double for inwxClient.
type fakeInwxClient struct {
	loginErr    error
	logoutCalls int

	createCalls []*goinwx.NameserverRecordRequest
	createErr   error

	infoResp *goinwx.NameserverInfoResponse
	infoErr  error

	deleteCalls []string
	deleteErr   error
}

func (f *fakeInwxClient) Login() error  { return f.loginErr }
func (f *fakeInwxClient) Logout() error { f.logoutCalls++; return nil }

func (f *fakeInwxClient) CreateRecord(req *goinwx.NameserverRecordRequest) (string, error) {
	f.createCalls = append(f.createCalls, req)
	return "1", f.createErr
}

func (f *fakeInwxClient) Info(_ *goinwx.NameserverInfoRequest) (*goinwx.NameserverInfoResponse, error) {
	return f.infoResp, f.infoErr
}

func (f *fakeInwxClient) DeleteRecord(id string) error {
	f.deleteCalls = append(f.deleteCalls, id)
	return f.deleteErr
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func buildSolver(fc *fakeInwxClient, objs ...k8sruntime.Object) *inwxDNSSolver {
	return &inwxDNSSolver{
		kubeClient: fake.NewSimpleClientset(objs...),
		newClient:  func(_, _ string) inwxClient { return fc },
	}
}

func testSecret(name, ns string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Data: map[string][]byte{
			"username": []byte("testuser"),
			"password": []byte("testpass"),
		},
	}
}

func testChallenge(ns, fqdn, zone, key string) *v1alpha1.ChallengeRequest {
	return &v1alpha1.ChallengeRequest{
		ResourceNamespace: ns,
		ResolvedFQDN:      fqdn,
		ResolvedZone:      zone,
		Key:               key,
		Config:            &extapi.JSON{Raw: []byte(`{"secretName":"inwx-credentials"}`)},
	}
}

// ---------------------------------------------------------------------------
// Present – happy path
// ---------------------------------------------------------------------------

func TestPresent_HappyPath(t *testing.T) {
	fc := &fakeInwxClient{infoResp: &goinwx.NameserverInfoResponse{}}
	s := buildSolver(fc, testSecret("inwx-credentials", "default"))
	ch := testChallenge("default", "_acme-challenge.www.example.com.", "example.com.", "token123")

	if err := s.Present(ch); err != nil {
		t.Fatalf("Present() unexpected error: %v", err)
	}

	if len(fc.createCalls) != 1 {
		t.Fatalf("expected 1 CreateRecord call, got %d", len(fc.createCalls))
	}
	req := fc.createCalls[0]
	if req.Domain != "example.com" {
		t.Errorf("domain: got %q, want %q", req.Domain, "example.com")
	}
	if req.Name != "_acme-challenge.www" {
		t.Errorf("name: got %q, want %q", req.Name, "_acme-challenge.www")
	}
	if req.Content != "token123" {
		t.Errorf("content: got %q, want %q", req.Content, "token123")
	}
	if req.Type != "TXT" {
		t.Errorf("type: got %q, want TXT", req.Type)
	}
	if req.TTL != 300 {
		t.Errorf("TTL: got %d, want 300", req.TTL)
	}
	if fc.logoutCalls != 1 {
		t.Errorf("expected Logout called once, got %d times", fc.logoutCalls)
	}
}

func TestPresent_ApexDomain(t *testing.T) {
	fc := &fakeInwxClient{infoResp: &goinwx.NameserverInfoResponse{}}
	s := buildSolver(fc, testSecret("inwx-credentials", "default"))
	// FQDN equals zone → record name should be "@"
	ch := testChallenge("default", "example.com.", "example.com.", "apextoken")

	if err := s.Present(ch); err != nil {
		t.Fatalf("Present() unexpected error for apex: %v", err)
	}
	if len(fc.createCalls) != 1 {
		t.Fatalf("expected 1 CreateRecord call, got %d", len(fc.createCalls))
	}
	if fc.createCalls[0].Name != "@" {
		t.Errorf("apex record name: got %q, want @", fc.createCalls[0].Name)
	}
}

func TestPresent_CustomSecretKeys(t *testing.T) {
	fc := &fakeInwxClient{infoResp: &goinwx.NameserverInfoResponse{}}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "inwx-credentials", Namespace: "default"},
		Data: map[string][]byte{
			"user": []byte("testuser"),
			"pass": []byte("testpass"),
		},
	}
	s := buildSolver(fc, secret)
	ch := &v1alpha1.ChallengeRequest{
		ResourceNamespace: "default",
		ResolvedFQDN:      "_acme-challenge.example.com.",
		ResolvedZone:      "example.com.",
		Key:               "token",
		Config:            &extapi.JSON{Raw: []byte(`{"secretName":"inwx-credentials","usernameKey":"user","passwordKey":"pass"}`)},
	}

	if err := s.Present(ch); err != nil {
		t.Fatalf("Present() unexpected error with custom key names: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Present – unhappy paths
// ---------------------------------------------------------------------------

func TestPresent_InvalidConfigJSON(t *testing.T) {
	fc := &fakeInwxClient{}
	s := buildSolver(fc)
	ch := &v1alpha1.ChallengeRequest{
		ResourceNamespace: "default",
		Config:            &extapi.JSON{Raw: []byte(`{not-valid-json}`)},
	}

	if err := s.Present(ch); err == nil {
		t.Fatal("expected error for invalid config JSON, got nil")
	}
}

func TestPresent_MissingSecretName(t *testing.T) {
	fc := &fakeInwxClient{}
	s := buildSolver(fc)
	ch := &v1alpha1.ChallengeRequest{
		ResourceNamespace: "default",
		ResolvedFQDN:      "_acme-challenge.example.com.",
		ResolvedZone:      "example.com.",
		Key:               "token",
		Config:            &extapi.JSON{Raw: []byte(`{}`)},
	}

	if err := s.Present(ch); err == nil {
		t.Fatal("expected error for missing secretName, got nil")
	}
}

func TestPresent_SecretNotFound(t *testing.T) {
	fc := &fakeInwxClient{}
	s := buildSolver(fc) // no secrets registered
	ch := testChallenge("default", "_acme-challenge.example.com.", "example.com.", "token")

	if err := s.Present(ch); err == nil {
		t.Fatal("expected error when secret does not exist, got nil")
	}
}

func TestPresent_MissingUsernameKey(t *testing.T) {
	fc := &fakeInwxClient{}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "inwx-credentials", Namespace: "default"},
		Data:       map[string][]byte{"password": []byte("pass")}, // "username" key absent
	}
	s := buildSolver(fc, secret)
	ch := testChallenge("default", "_acme-challenge.example.com.", "example.com.", "token")

	if err := s.Present(ch); err == nil {
		t.Fatal("expected error for missing username key, got nil")
	}
}

func TestPresent_MissingPasswordKey(t *testing.T) {
	fc := &fakeInwxClient{}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "inwx-credentials", Namespace: "default"},
		Data:       map[string][]byte{"username": []byte("user")}, // "password" key absent
	}
	s := buildSolver(fc, secret)
	ch := testChallenge("default", "_acme-challenge.example.com.", "example.com.", "token")

	if err := s.Present(ch); err == nil {
		t.Fatal("expected error for missing password key, got nil")
	}
}

func TestPresent_LoginFails(t *testing.T) {
	fc := &fakeInwxClient{loginErr: errors.New("invalid credentials")}
	s := buildSolver(fc, testSecret("inwx-credentials", "default"))
	ch := testChallenge("default", "_acme-challenge.example.com.", "example.com.", "token")

	if err := s.Present(ch); err == nil {
		t.Fatal("expected error on login failure, got nil")
	}
}

func TestPresent_CreateRecordFails(t *testing.T) {
	fc := &fakeInwxClient{createErr: errors.New("API error")}
	s := buildSolver(fc, testSecret("inwx-credentials", "default"))
	ch := testChallenge("default", "_acme-challenge.example.com.", "example.com.", "token")

	if err := s.Present(ch); err == nil {
		t.Fatal("expected error on CreateRecord failure, got nil")
	}
}

// ---------------------------------------------------------------------------
// CleanUp – happy path
// ---------------------------------------------------------------------------

func TestCleanUp_HappyPath(t *testing.T) {
	fc := &fakeInwxClient{
		infoResp: &goinwx.NameserverInfoResponse{
			Records: []goinwx.NameserverRecord{
				{ID: "42", Content: "token123"},
				{ID: "99", Content: "other-token"},
			},
		},
	}
	s := buildSolver(fc, testSecret("inwx-credentials", "default"))
	ch := testChallenge("default", "_acme-challenge.www.example.com.", "example.com.", "token123")

	if err := s.CleanUp(ch); err != nil {
		t.Fatalf("CleanUp() unexpected error: %v", err)
	}

	// Only the matching record (ID 42) should be deleted.
	if len(fc.deleteCalls) != 1 {
		t.Fatalf("expected 1 DeleteRecord call, got %d", len(fc.deleteCalls))
	}
	if fc.deleteCalls[0] != "42" {
		t.Errorf("expected record ID 42 to be deleted, got %s", fc.deleteCalls[0])
	}
	if fc.logoutCalls != 1 {
		t.Errorf("expected Logout called once, got %d times", fc.logoutCalls)
	}
}

func TestCleanUp_NoMatchingRecord(t *testing.T) {
	fc := &fakeInwxClient{
		infoResp: &goinwx.NameserverInfoResponse{
			Records: []goinwx.NameserverRecord{
				{ID: "99", Content: "different-token"},
			},
		},
	}
	s := buildSolver(fc, testSecret("inwx-credentials", "default"))
	ch := testChallenge("default", "_acme-challenge.www.example.com.", "example.com.", "token123")

	if err := s.CleanUp(ch); err != nil {
		t.Fatalf("CleanUp() unexpected error: %v", err)
	}
	if len(fc.deleteCalls) != 0 {
		t.Errorf("expected no DeleteRecord calls for non-matching record, got %d", len(fc.deleteCalls))
	}
}

func TestCleanUp_EmptyRecordList(t *testing.T) {
	fc := &fakeInwxClient{
		infoResp: &goinwx.NameserverInfoResponse{Records: nil},
	}
	s := buildSolver(fc, testSecret("inwx-credentials", "default"))
	ch := testChallenge("default", "_acme-challenge.example.com.", "example.com.", "token")

	if err := s.CleanUp(ch); err != nil {
		t.Fatalf("CleanUp() unexpected error on empty record list: %v", err)
	}
	if len(fc.deleteCalls) != 0 {
		t.Errorf("expected no DeleteRecord calls, got %d", len(fc.deleteCalls))
	}
}

func TestCleanUp_DeletesMultipleMatchingRecords(t *testing.T) {
	// Duplicates may exist; all matches should be removed.
	fc := &fakeInwxClient{
		infoResp: &goinwx.NameserverInfoResponse{
			Records: []goinwx.NameserverRecord{
				{ID: "10", Content: "token"},
				{ID: "11", Content: "token"},
				{ID: "12", Content: "other"},
			},
		},
	}
	s := buildSolver(fc, testSecret("inwx-credentials", "default"))
	ch := testChallenge("default", "_acme-challenge.example.com.", "example.com.", "token")

	if err := s.CleanUp(ch); err != nil {
		t.Fatalf("CleanUp() unexpected error: %v", err)
	}
	if len(fc.deleteCalls) != 2 {
		t.Fatalf("expected 2 DeleteRecord calls, got %d", len(fc.deleteCalls))
	}
}

// ---------------------------------------------------------------------------
// CleanUp – unhappy paths
// ---------------------------------------------------------------------------

func TestCleanUp_InvalidConfigJSON(t *testing.T) {
	fc := &fakeInwxClient{}
	s := buildSolver(fc)
	ch := &v1alpha1.ChallengeRequest{
		ResourceNamespace: "default",
		Config:            &extapi.JSON{Raw: []byte(`{not-valid}`)},
	}

	if err := s.CleanUp(ch); err == nil {
		t.Fatal("expected error for invalid config JSON, got nil")
	}
}

func TestCleanUp_SecretNotFound(t *testing.T) {
	fc := &fakeInwxClient{}
	s := buildSolver(fc) // no secrets registered
	ch := testChallenge("default", "_acme-challenge.example.com.", "example.com.", "token")

	if err := s.CleanUp(ch); err == nil {
		t.Fatal("expected error when secret does not exist, got nil")
	}
}

func TestCleanUp_LoginFails(t *testing.T) {
	fc := &fakeInwxClient{loginErr: errors.New("auth error")}
	s := buildSolver(fc, testSecret("inwx-credentials", "default"))
	ch := testChallenge("default", "_acme-challenge.example.com.", "example.com.", "token")

	if err := s.CleanUp(ch); err == nil {
		t.Fatal("expected error when login fails, got nil")
	}
}

func TestCleanUp_InfoFails(t *testing.T) {
	fc := &fakeInwxClient{infoErr: errors.New("nameserver error")}
	s := buildSolver(fc, testSecret("inwx-credentials", "default"))
	ch := testChallenge("default", "_acme-challenge.example.com.", "example.com.", "token")

	if err := s.CleanUp(ch); err == nil {
		t.Fatal("expected error when Info fails, got nil")
	}
}

func TestCleanUp_DeleteFails(t *testing.T) {
	fc := &fakeInwxClient{
		infoResp: &goinwx.NameserverInfoResponse{
			Records: []goinwx.NameserverRecord{{ID: "1", Content: "token"}},
		},
		deleteErr: errors.New("delete failed"),
	}
	s := buildSolver(fc, testSecret("inwx-credentials", "default"))
	ch := testChallenge("default", "_acme-challenge.example.com.", "example.com.", "token")

	if err := s.CleanUp(ch); err == nil {
		t.Fatal("expected error when DeleteRecord fails, got nil")
	}
}
