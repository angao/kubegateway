package controllers

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	requestx509 "k8s.io/apiserver/pkg/authentication/request/x509"
	"k8s.io/apiserver/pkg/server/dynamiccertificates"
	"k8s.io/klog"
	"sigs.k8s.io/yaml"

	proxyv1alpha1 "github.com/kubewharf/kubegateway/pkg/apis/proxy/v1alpha1"
	"github.com/kubewharf/kubegateway/pkg/clusters"
	gatewaynet "github.com/kubewharf/kubegateway/pkg/gateway/net"
)

var _ dynamiccertificates.DynamicClientConfigProvider = &UpstreamClusterManager{}
var _ requestx509.SNIVerifyOptionsProvider = &UpstreamClusterManager{}

type UpstreamClusterManager struct {
	path string
	clusters.Manager
}

// NewUpstreamClusterManager returns a UpstreamClusterManager
func NewUpstreamClusterManager(path string) *UpstreamClusterManager {
	return &UpstreamClusterManager{
		path:    path,
		Manager: clusters.NewManager(),
	}
}

func (m *UpstreamClusterManager) Run() {
	klog.Infof("start to read upstream cluster file")
	content, err := os.ReadFile(m.path)
	if err != nil {
		panic(fmt.Errorf("read upstream cluster file failed: %v", err))
	}

	cluster := &proxyv1alpha1.UpstreamCluster{}
	if err := yaml.Unmarshal(content, cluster); err != nil {
		panic(fmt.Errorf("yaml unmarshal failed: %v", err))
	}

	clusterInfo, err := clusters.CreateClusterInfo(cluster, GatewayHealthCheck)
	if err != nil {
		klog.Errorf("failed to create cluster: %v, err: %v", cluster.Name, err)
		return
	}
	m.Add(clusterInfo)
}

func (m *UpstreamClusterManager) WrapGetConfigForClient(getConfigFunc dynamiccertificates.GetConfigForClientFunc) dynamiccertificates.GetConfigForClientFunc {
	return func(clientHello *tls.ClientHelloInfo) (*tls.Config, error) {
		baseTLSConfig, err := getConfigFunc(clientHello)
		if err != nil {
			return baseTLSConfig, err
		}

		// if the client set SNI information, just use our "normal" SNI flow
		// Get request host name from SNI information or inspect the requested IP
		hostname := clientHello.ServerName
		if len(hostname) == 0 {
			// if the client didn't set SNI, then we need to inspect the requested IP so that we can choose
			// a certificate from our list if we specifically handle that IP.  This can happen when an IP is specifically mapped by name.
			var err error
			hostname, _, err = net.SplitHostPort(clientHello.Conn.LocalAddr().String())
			if err != nil {
				klog.Errorf("failed to get hostname from clientHello's conn: %v", err)
				return baseTLSConfig, nil
			}
		}

		klog.V(5).Infof("get tls config for %q", hostname)

		cluster, ok := m.Get(hostname)
		if !ok {
			return baseTLSConfig, nil
		}

		tlsConfig, ok := cluster.LoadTLSConfig()
		if !ok {
			return baseTLSConfig, nil
		}

		tlsConfigCopy := baseTLSConfig.Clone()

		if tlsConfig.ClientCAs != nil {
			// Populate PeerCertificates in requests, but don't reject connections without certificates
			// This allows certificates to be validated by authenticators, while still allowing other auth types
			tlsConfigCopy.ClientAuth = tls.RequestClientCert
			tlsConfigCopy.ClientCAs = tlsConfig.ClientCAs
		}
		if len(tlsConfig.Certificates) > 0 {
			// provide specific certificates
			tlsConfigCopy.Certificates = tlsConfig.Certificates
			// tlsConfigCopy.NameToCertificate = nil //nolint
			tlsConfigCopy.GetCertificate = nil
			tlsConfigCopy.GetConfigForClient = nil
		}
		return tlsConfigCopy, nil
	}
}

func (m *UpstreamClusterManager) SNIVerifyOptions(host string) (x509.VerifyOptions, bool) {
	hostname := gatewaynet.HostWithoutPort(host)
	empty := x509.VerifyOptions{}
	cluster, ok := m.Get(hostname)
	if !ok {
		return empty, false
	}
	return cluster.LoadVerifyOptions()
}

// GatewayHealthCheck health check endpoint periodically
func GatewayHealthCheck(e *clusters.EndpointInfo) (done bool) {
	done = false

	// TODO: use readyz if all kubernetes master version is greater than v1.16
	result := e.Clientset().CoreV1().RESTClient().
		Get().AbsPath("/readyz").Timeout(5 * time.Second).Do(context.TODO())
	err := result.Error()

	var reason, message string
	statusCode := 0

	if err != nil {
		if os.IsTimeout(err) {
			reason = "Timeout"
			message = err.Error()
		} else {
			switch status := err.(type) {
			case errors.APIStatus:
				reason = string(status.Status().Reason)
				message = status.Status().Message
			default:
				reason = "Failure"
				message = err.Error()
			}
		}
	} else {
		result.StatusCode(&statusCode)
		if statusCode == http.StatusOK {
			e.UpdateStatus(true, "", "")
			return done
		}
		reason = "NotReady"
		message = fmt.Sprintf("request %s/readyz, got response code is %v", e.Endpoint, statusCode)
	}
	klog.Errorf("upstream health check failed, cluster=%q endpoint=%q reason=%q message=%q", e.Cluster, e.Endpoint, reason, message)
	e.UpdateStatus(false, reason, message)
	return done
}
