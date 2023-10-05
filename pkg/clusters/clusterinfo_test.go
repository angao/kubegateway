// Copyright 2022 ByteDance and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package clusters

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math"
	"math/big"
	"testing"
	"time"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/keyutil"

	proxyv1alpha1 "github.com/kubewharf/kubegateway/pkg/apis/proxy/v1alpha1"
	"github.com/kubewharf/kubegateway/pkg/flowcontrol"
)

func createCAandCert() (serverKey []byte, serverCert []byte, caCert []byte) {
	caKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	ca, err := cert.NewSelfSignedCACert(cert.Config{
		CommonName: "test",
	}, caKey)
	if err != nil {
		panic(err)
	}

	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	crt, err := NewSignedCert(&cert.Config{CommonName: "server"}, key, ca, caKey)
	if err != nil {
		panic(err)
	}
	caPEM := &pem.Block{Type: cert.CertificateBlockType, Bytes: ca.Raw}
	keyPEM := &pem.Block{Type: keyutil.RSAPrivateKeyBlockType, Bytes: x509.MarshalPKCS1PrivateKey(key)}
	crtPEM := &pem.Block{Type: cert.CertificateBlockType, Bytes: crt.Raw}
	return pem.EncodeToMemory(keyPEM), pem.EncodeToMemory(crtPEM), pem.EncodeToMemory(caPEM)
}

// NewSignedCert creates a signed certificate using the given CA certificate and key
func NewSignedCert(cfg *cert.Config, key crypto.Signer, caCert *x509.Certificate, caKey crypto.Signer) (*x509.Certificate, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).SetInt64(math.MaxInt64))
	if err != nil {
		return nil, err
	}
	if len(cfg.CommonName) == 0 {
		return nil, errors.New("must specify a CommonName")
	}

	certTmpl := x509.Certificate{
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: cfg.Organization,
		},
		DNSNames:     cfg.AltNames.DNSNames,
		IPAddresses:  cfg.AltNames.IPs,
		SerialNumber: serial,
		NotBefore:    caCert.NotBefore,
		NotAfter:     time.Now().Add(365 * 24 * time.Hour).UTC(),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  cfg.Usages,
	}
	certDERBytes, err := x509.CreateCertificate(rand.Reader, &certTmpl, caCert, key.Public(), caKey)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(certDERBytes)
}

func newTestUpstreamClusterConfig() *proxyv1alpha1.UpstreamCluster {
	key, crt, ca := createCAandCert()

	return &proxyv1alpha1.UpstreamCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testing.cluster",
		},
		Spec: proxyv1alpha1.UpstreamClusterSpec{
			Servers: []proxyv1alpha1.UpstreamClusterServer{
				{
					Endpoint: "https://127.0.0.1:443",
				},
			},
			ClientConfig: proxyv1alpha1.ClientConfig{
				Insecure:    true,
				BearerToken: []byte("aaaa"),
				QPS:         10,
				Burst:       20,
			},
			SecureServing: proxyv1alpha1.SecureServing{
				KeyData:      key,
				CertData:     crt,
				ClientCAData: ca,
			},
			DispatchPolicies: []proxyv1alpha1.DispatchPolicy{
				{
					Rules: []proxyv1alpha1.DispatchPolicyRule{
						{
							Verbs:           []string{"*"},
							APIGroups:       []string{"*"},
							Resources:       []string{"*"},
							NonResourceURLs: []string{"*"},
						},
					},
				},
			},
		},
	}
}

func createTestClusterInfo() *ClusterInfo {
	ret, _ := CreateClusterInfo(newTestUpstreamClusterConfig(), alwaysReadyHealthCheck)
	return ret
}

func alwaysReadyHealthCheck(e *EndpointInfo) (done bool) {
	done = false
	if e.IsReady() {
		return
	}
	e.UpdateStatus(true, "", "")
	return
}

func TestClusterInfo_syncEndpoints(t *testing.T) {
	type args struct {
		clusterInfo *ClusterInfo
		servers     []proxyv1alpha1.UpstreamClusterServer
	}
	tests := []struct {
		name    string
		args    args
		want    sets.String
		wantErr bool
	}{
		{
			"add endpoint",
			args{
				clusterInfo: createTestClusterInfo(),
				servers: []proxyv1alpha1.UpstreamClusterServer{
					{
						Endpoint: "https://127.0.0.1:443",
					},
					{
						Endpoint: "https://127.0.0.2:443",
					},
				},
			},
			sets.NewString("https://127.0.0.1:443", "https://127.0.0.2:443"),
			false,
		},
		{
			"delete endpoint",
			args{
				clusterInfo: createTestClusterInfo(),
				servers: []proxyv1alpha1.UpstreamClusterServer{
					{
						Endpoint: "https://127.0.0.2:443",
					},
				},
			},
			sets.NewString("https://127.0.0.2:443"),
			false,
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			err := tt.args.clusterInfo.syncEndpoints(tt.args.servers)
			if (err != nil) != tt.wantErr {
				t.Errorf("ClusterInfo.syncEndpoints() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			got := sets.NewString(tt.args.clusterInfo.AllEndpoints()...)
			if !got.Equal(tt.want) {
				t.Errorf("ClusterInfo.syncEndpoints() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClusterInfo_syncSecureServingConfigLocked(t *testing.T) {
	type args struct {
		clusterInfo   *ClusterInfo
		secureServing proxyv1alpha1.SecureServing
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
		check   func(info *ClusterInfo) error
	}{
		{
			"empty secure serving",
			args{
				clusterInfo:   createTestClusterInfo(),
				secureServing: proxyv1alpha1.SecureServing{},
			},
			false,
			func(info *ClusterInfo) error {
				_, ok := info.LoadVerifyOptions()
				if ok {
					return fmt.Errorf("verify options should be nil")
				}
				_, ok = info.LoadTLSConfig()
				if ok {
					return fmt.Errorf("tls config should be nil")
				}
				return nil
			},
		},
		{
			"replace serving key and crt",
			args{
				clusterInfo: createTestClusterInfo(),
				secureServing: func() proxyv1alpha1.SecureServing {
					key, crt, _ := createCAandCert()
					return proxyv1alpha1.SecureServing{
						KeyData:  key,
						CertData: crt,
					}
				}(),
			},
			false,
			func(info *ClusterInfo) error {
				_, ok := info.LoadVerifyOptions()
				if ok {
					return fmt.Errorf("verify options should be nil")
				}
				tlsConfig, ok := info.LoadTLSConfig()
				if !ok {
					return fmt.Errorf("tls config should not be nil")
				}
				if tlsConfig.ClientCAs != nil {
					return fmt.Errorf("tlsConfig.ClientCAs should be nil")
				}
				if len(tlsConfig.Certificates) == 0 {
					return fmt.Errorf("tlsConfig.Certificates should not be nil")
				}
				return nil
			},
		},
		{
			"replace ca",
			args{
				clusterInfo: createTestClusterInfo(),
				secureServing: func() proxyv1alpha1.SecureServing {
					_, _, ca := createCAandCert()
					return proxyv1alpha1.SecureServing{
						ClientCAData: ca,
					}
				}(),
			},
			false,
			func(info *ClusterInfo) error {
				_, ok := info.LoadVerifyOptions()
				if !ok {
					return fmt.Errorf("verify options should not be nil")
				}
				tlsConfig, ok := info.LoadTLSConfig()
				if !ok {
					return fmt.Errorf("tls config should not be nil")
				}
				if tlsConfig.ClientCAs == nil {
					return fmt.Errorf("tlsConfig.ClientCAs should not be nil")
				}
				if len(tlsConfig.Certificates) > 0 {
					return fmt.Errorf("tlsConfig.Certificates should be nil")
				}
				return nil
			},
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			err := tt.args.clusterInfo.syncSecureServingConfigLocked(tt.args.secureServing)
			if (err != nil) != tt.wantErr {
				t.Errorf("ClusterInfo.syncSecureServingConfigLocked() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.check != nil {
				if err := tt.check(tt.args.clusterInfo); err != nil {
					t.Errorf("ClusterInfo.syncSecureServingConfigLocked() error = %v", err)
				}
			}
		})
	}
}

func TestClusterInfo_syncFlowControlLocked(t *testing.T) {
	exempt := proxyv1alpha1.FlowControlSchema{
		Name: "exempt",
		FlowControlSchemaConfiguration: proxyv1alpha1.FlowControlSchemaConfiguration{
			Exempt: &proxyv1alpha1.ExemptFlowControlSchema{},
		},
	}
	maxInflight10 := proxyv1alpha1.FlowControlSchema{
		Name: "max-inflight",
		FlowControlSchemaConfiguration: proxyv1alpha1.FlowControlSchemaConfiguration{
			MaxRequestsInflight: &proxyv1alpha1.MaxRequestsInflightFlowControlSchema{
				Max: 10,
			},
		},
	}
	tokenBucket10 := proxyv1alpha1.FlowControlSchema{
		Name: "tokenbucket",
		FlowControlSchemaConfiguration: proxyv1alpha1.FlowControlSchemaConfiguration{
			TokenBucket: &proxyv1alpha1.TokenBucketFlowControlSchema{
				QPS:   10,
				Burst: 20,
			},
		},
	}
	maxInflight20 := proxyv1alpha1.FlowControlSchema{
		Name: "max-inflight",
		FlowControlSchemaConfiguration: proxyv1alpha1.FlowControlSchemaConfiguration{
			MaxRequestsInflight: &proxyv1alpha1.MaxRequestsInflightFlowControlSchema{
				Max: 20,
			},
		},
	}
	tokenBucket20 := proxyv1alpha1.FlowControlSchema{
		Name: "tokenbucket",
		FlowControlSchemaConfiguration: proxyv1alpha1.FlowControlSchemaConfiguration{
			TokenBucket: &proxyv1alpha1.TokenBucketFlowControlSchema{
				QPS:   20,
				Burst: 40,
			},
		},
	}
	type args struct {
		clusterInfo *ClusterInfo
		oldObj      proxyv1alpha1.FlowControl
		newObj      proxyv1alpha1.FlowControl
	}
	tests := []struct {
		name  string
		args  args
		check func(info *ClusterInfo) error
	}{
		{
			name: "add new flow control",
			args: args{
				clusterInfo: createTestClusterInfo(),
				newObj: proxyv1alpha1.FlowControl{
					Schemas: []proxyv1alpha1.FlowControlSchema{
						exempt,
						maxInflight10,
						tokenBucket10,
					},
				},
			},
			check: func(info *ClusterInfo) error {
				_, ok := info.flowcontrol.Load("exempt")
				if !ok {
					return fmt.Errorf("missing exempt flowcontrol")
				}
				_, ok = info.flowcontrol.Load("max-inflight")
				if !ok {
					return fmt.Errorf("missing max-inflight flowcontrol")
				}
				_, ok = info.flowcontrol.Load("tokenbucket")
				if !ok {
					return fmt.Errorf("missing tokenbucket flowcontrol")
				}
				return nil
			},
		},
		{
			name: "add new flow control",
			args: args{
				clusterInfo: createTestClusterInfo(),
				oldObj: proxyv1alpha1.FlowControl{
					Schemas: []proxyv1alpha1.FlowControlSchema{
						exempt,
						maxInflight10,
						tokenBucket10,
					},
				},
			},
			check: func(info *ClusterInfo) error {
				if info.flowcontrol.Len() > 0 {
					return fmt.Errorf("flow controls are not deleted")
				}
				return nil
			},
		},
		{
			name: "resize",
			args: args{
				clusterInfo: createTestClusterInfo(),
				oldObj: proxyv1alpha1.FlowControl{
					Schemas: []proxyv1alpha1.FlowControlSchema{
						maxInflight10,
						tokenBucket10,
					},
				},
				newObj: proxyv1alpha1.FlowControl{
					Schemas: []proxyv1alpha1.FlowControlSchema{
						maxInflight20,
						tokenBucket20,
					},
				},
			},
			check: func(info *ClusterInfo) error {
				fl, _ := info.flowcontrol.Load(maxInflight10.Name)
				got := fl.String()
				want := flowcontrol.NewFlowControl(maxInflight20).String()
				if got != want {
					return fmt.Errorf("max-inflight is not resized, got=%v, want=%v", got, want)
				}
				fl, _ = info.flowcontrol.Load(tokenBucket10.Name)
				got = fl.String()
				want = flowcontrol.NewFlowControl(tokenBucket20).String()
				if got != want {
					return fmt.Errorf("tokenbucket is not resized, got=%v, want=%v", got, want)
				}
				return nil
			},
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			tt.args.clusterInfo.syncFlowControlLocked(tt.args.oldObj)
			tt.args.clusterInfo.syncFlowControlLocked(tt.args.newObj)
			if tt.check != nil {
				if err := tt.check(tt.args.clusterInfo); err != nil {
					t.Errorf("ClusterInfo.syncFlowControlLocked() error = %v", err)
				}
			}
		})
	}
}

func TestClusterInfo_sync(t *testing.T) {
	a := proxyv1alpha1.SecureServing{}
	b := proxyv1alpha1.SecureServing{}
	equal := apiequality.Semantic.DeepEqual(a, b)
	if equal != true {
		t.Errorf("should be equal")
	}

	a = proxyv1alpha1.SecureServing{
		KeyData: []byte("12345"),
	}
	b = proxyv1alpha1.SecureServing{
		KeyData: []byte("12345"),
	}
	equal = apiequality.Semantic.DeepEqual(a, b)
	if equal != true {
		t.Errorf("should be equal")
	}
}

func Test_isLogEnabled(t *testing.T) {
	tests := []struct {
		name     string
		upstream proxyv1alpha1.LogMode
		policy   proxyv1alpha1.LogMode
		want     bool
	}{
		{
			"upstream off",
			proxyv1alpha1.LogOff,
			proxyv1alpha1.LogOn,
			false,
		},
		{
			"policy off",
			proxyv1alpha1.LogOn,
			proxyv1alpha1.LogOff,
			false,
		},
		{
			"empty",
			"",
			"",
			false,
		},
		{
			"upstream on",
			proxyv1alpha1.LogOn,
			"",
			true,
		},
		{
			"policy on",
			"",
			proxyv1alpha1.LogOn,
			true,
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			if got := isLogEnabled(tt.upstream, tt.policy); got != tt.want {
				t.Errorf("isLogEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}
