// Copyright 2024 The Carvel Authors.
// SPDX-License-Identifier: Apache-2.0

package http_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	ctlconf "carvel.dev/vendir/pkg/vendir/config"
	vendirhttp "carvel.dev/vendir/pkg/vendir/fetch/http"
	"github.com/stretchr/testify/require"
)

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
	if f.configMaps == nil {
		return ctlconf.ConfigMap{}, fmt.Errorf("configmap %q not found", name)
	}

	cm, ok := f.configMaps[name]
	if !ok {
		return ctlconf.ConfigMap{}, fmt.Errorf("configmap %q not found", name)
	}
	return cm, nil
}

type fakeTempArea struct {
	baseDir string
}

func (f fakeTempArea) NewTempDir(prefix string) (string, error) {
	return os.MkdirTemp(f.baseDir, prefix)
}

func (f fakeTempArea) NewTempFile(prefix string) (*os.File, error) {
	return os.CreateTemp(f.baseDir, prefix)
}

func secretRef(name string) *ctlconf.DirectoryContentsLocalRef {
	return &ctlconf.DirectoryContentsLocalRef{Name: name}
}

type syncTest struct {
	name          string
	secret        ctlconf.Secret
	expectedBody  string
	expectedError string
	validateReq   func(t *testing.T, r *http.Request)
}

func TestSync_HTTPAuth(t *testing.T) {
	allTests := []syncTest{
		{
			name: "when basic auth username and password are provided, it succeeds",
			secret: ctlconf.Secret{
				Metadata: ctlconf.GenericMetadata{Name: "http-auth"},
				Data: map[string][]byte{
					ctlconf.SecretK8sCorev1BasicAuthUsernameKey: []byte("admin"),
					ctlconf.SecretK8sCorev1BasicAuthPasswordKey: []byte("password"),
				},
			},
			expectedBody: "ok",
			validateReq: func(t *testing.T, r *http.Request) {
				user, pass, ok := r.BasicAuth()
				require.True(t, ok)
				require.Equal(t, "admin", user)
				require.Equal(t, "password", pass)
			},
		},
		{
			name: "when bearer token is provided, it succeeds",
			secret: ctlconf.Secret{
				Metadata: ctlconf.GenericMetadata{Name: "http-auth"},
				Data: map[string][]byte{
					ctlconf.SecretK8sCorev1HTTPBearerTokenKey: []byte("abc123"),
				},
			},
			expectedBody: "ok",
			validateReq: func(t *testing.T, r *http.Request) {
				require.Equal(t, "Bearer abc123", r.Header.Get("Authorization"))
			},
		},
		{
			name: "when username is provided without password, it uses empty password and succeeds",
			secret: ctlconf.Secret{
				Metadata: ctlconf.GenericMetadata{Name: "http-auth"},
				Data: map[string][]byte{
					ctlconf.SecretK8sCorev1BasicAuthUsernameKey: []byte("admin"),
				},
			},
			expectedBody: "ok",
			validateReq: func(t *testing.T, r *http.Request) {
				user, pass, ok := r.BasicAuth()
				require.True(t, ok)
				require.Equal(t, "admin", user)
				require.Equal(t, "", pass)
			},
		},
		{
			name: "when basic auth and bearer token are mixed, it fails",
			secret: ctlconf.Secret{
				Metadata: ctlconf.GenericMetadata{Name: "http-auth"},
				Data: map[string][]byte{
					ctlconf.SecretK8sCorev1BasicAuthUsernameKey: []byte("admin"),
					ctlconf.SecretK8sCorev1BasicAuthPasswordKey: []byte("password"),
					ctlconf.SecretK8sCorev1HTTPBearerTokenKey:   []byte("abc123"),
				},
			},
			expectedError: "must not contain both basic auth",
		},
		{
			name: "when password is provided without username, it fails",
			secret: ctlconf.Secret{
				Metadata: ctlconf.GenericMetadata{Name: "http-auth"},
				Data: map[string][]byte{
					ctlconf.SecretK8sCorev1BasicAuthPasswordKey: []byte("password"),
				},
			},
			expectedError: "is missing 'username'",
		},
	}

	for _, test := range allTests {
		t.Run(test.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if test.expectedError != "" {
					t.Fatalf("server should not be reached when auth setup fails")
				}

				test.validateReq(t, r)

				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte(test.expectedBody))
				require.NoError(t, err)
			}))
			defer srv.Close()

			ref := fakeRefFetcher{
				secrets: map[string]ctlconf.Secret{
					"http-auth": test.secret,
				},
			}

			subject := vendirhttp.NewSync(ctlconf.DirectoryContentsHTTP{
				URL:           srv.URL,
				SecretRef:     secretRef("http-auth"),
				DisableUnpack: true,
			}, ref)

			tempRoot, err := os.MkdirTemp("", "vendir-http-test")
			require.NoError(t, err)
			defer os.RemoveAll(tempRoot)

			dstPath := filepath.Join(tempRoot, "dst")
			_, err = subject.Sync(dstPath, fakeTempArea{baseDir: tempRoot})

			if test.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), test.expectedError)
				return
			}

			require.NoError(t, err)

			bs, err := os.ReadFile(filepath.Join(dstPath, filepath.Base(srv.URL)))
			require.NoError(t, err)
			require.Equal(t, test.expectedBody, string(bs))
		})
	}
}
