package http

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	ctlconf "carvel.dev/vendir/pkg/vendir/config"
)

/*
Fake RefFetcher (implements ctlfetch.RefFetcher)
*/

type fakeRefFetcher struct {
	secrets    map[string]ctlconf.Secret
	configMaps map[string]ctlconf.ConfigMap
}

func (f fakeRefFetcher) GetSecret(name string) (ctlconf.Secret, error) {
	s, ok := f.secrets[name]
	if !ok {
		return ctlconf.Secret{}, fmt.Errorf("secret %q not found", name)
	}
	return s, nil
}

func (f fakeRefFetcher) GetConfigMap(name string) (ctlconf.ConfigMap, error) {
	// Not used by these tests, but required by the interface.
	if f.configMaps == nil {
		return ctlconf.ConfigMap{}, fmt.Errorf("configmap %q not found", name)
	}
	cm, ok := f.configMaps[name]
	if !ok {
		return ctlconf.ConfigMap{}, fmt.Errorf("configmap %q not found", name)
	}
	return cm, nil
}

/*
Helper: build the correct SecretRef type for DirectoryContentsHTTP.
In vendir, HTTP uses a dedicated type (not a generic ctlconf.SecretRef).
*/
func secretRef(name string) *ctlconf.DirectoryContentsLocalRef {
	return &ctlconf.DirectoryContentsLocalRef{Name: name}
}

/*
Tests
*/

func TestHTTPAuth_BasicAuth_Succeeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "admin" || pass != "password" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	ref := fakeRefFetcher{
		secrets: map[string]ctlconf.Secret{
			"http-auth": {
				Metadata: ctlconf.GenericMetadata{Name: "http-auth"},
				Data: map[string][]byte{
					ctlconf.SecretK8sCorev1BasicAuthUsernameKey: []byte("admin"),
					ctlconf.SecretK8sCorev1BasicAuthPasswordKey: []byte("password"),
				},
			},
		},
	}

	s := NewSync(ctlconf.DirectoryContentsHTTP{
		URL:       srv.URL,
		SecretRef: secretRef("http-auth"),
	}, ref)

	var dst bytes.Buffer
	if err := s.downloadFile(&dst); err != nil {
		t.Fatalf("expected basic auth download to succeed, got error: %v", err)
	}
}

func TestHTTPAuth_BearerToken_Succeeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer abc123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	ref := fakeRefFetcher{
		secrets: map[string]ctlconf.Secret{
			"http-auth": {
				Metadata: ctlconf.GenericMetadata{Name: "http-auth"},
				Data: map[string][]byte{
					ctlconf.SecretK8sCorev1HTTPBearerTokenKey: []byte("abc123"),
				},
			},
		},
	}

	s := NewSync(ctlconf.DirectoryContentsHTTP{
		URL:       srv.URL,
		SecretRef: secretRef("http-auth"),
	}, ref)

	var dst bytes.Buffer
	if err := s.downloadFile(&dst); err != nil {
		t.Fatalf("expected bearer auth download to succeed, got error: %v", err)
	}
}

func TestHTTPAuth_MixedAuth_Fails(t *testing.T) {
	ref := fakeRefFetcher{
		secrets: map[string]ctlconf.Secret{
			"http-auth": {
				Metadata: ctlconf.GenericMetadata{Name: "http-auth"},
				Data: map[string][]byte{
					ctlconf.SecretK8sCorev1BasicAuthUsernameKey: []byte("admin"),
					ctlconf.SecretK8sCorev1BasicAuthPasswordKey: []byte("password"),
					ctlconf.SecretK8sCorev1HTTPBearerTokenKey:   []byte("abc123"),
				},
			},
		},
	}

	s := NewSync(ctlconf.DirectoryContentsHTTP{
		URL:       "http://example.com",
		SecretRef: secretRef("http-auth"),
	}, ref)

	req, _ := http.NewRequest("GET", "http://example.com", nil)
	if err := s.addAuth(req); err == nil {
		t.Fatalf("expected error for mixed auth, got nil")
	}
}

func TestHTTPAuth_UsernameWithoutPassword_Fails(t *testing.T) {
	ref := fakeRefFetcher{
		secrets: map[string]ctlconf.Secret{
			"http-auth": {
				Metadata: ctlconf.GenericMetadata{Name: "http-auth"},
				Data: map[string][]byte{
					ctlconf.SecretK8sCorev1BasicAuthUsernameKey: []byte("admin"),
				},
			},
		},
	}

	s := NewSync(ctlconf.DirectoryContentsHTTP{
		URL:       "http://example.com",
		SecretRef: secretRef("http-auth"),
	}, ref)

	req, _ := http.NewRequest("GET", "http://example.com", nil)
	if err := s.addAuth(req); err == nil {
		t.Fatalf("expected error when username is set without password")
	}
}

func TestHTTPAuth_PasswordWithoutUsername_Fails(t *testing.T) {
	ref := fakeRefFetcher{
		secrets: map[string]ctlconf.Secret{
			"http-auth": {
				Metadata: ctlconf.GenericMetadata{Name: "http-auth"},
				Data: map[string][]byte{
					ctlconf.SecretK8sCorev1BasicAuthPasswordKey: []byte("password"),
				},
			},
		},
	}

	s := NewSync(ctlconf.DirectoryContentsHTTP{
		URL:       "http://example.com",
		SecretRef: secretRef("http-auth"),
	}, ref)

	req, _ := http.NewRequest("GET", "http://example.com", nil)
	if err := s.addAuth(req); err == nil {
		t.Fatalf("expected error when password is set without username")
	}
}
