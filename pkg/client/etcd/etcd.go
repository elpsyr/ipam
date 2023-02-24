package etcd

import (
	"go.etcd.io/etcd/client/pkg/v3/transport"
	client3 "go.etcd.io/etcd/client/v3"
	"time"
)

// we use  "go.etcd.io/etcd/client/v3" to create a client to operating etcd

const (
	clientTimeout = 30 * time.Second
)

type Connection struct {
	EtcdEndpoints  string
	EtcdCertFile   string
	EtcdKeyFile    string
	EtcdCACertFile string
}

func GetClient(conn *Connection) (*client3.Client, error) {
	tlsInfo := transport.TLSInfo{
		CertFile:      conn.EtcdCertFile,
		KeyFile:       conn.EtcdKeyFile,
		TrustedCAFile: conn.EtcdCACertFile,
	}
	tlsConfig, err := tlsInfo.ClientConfig()
	if err != nil {
		return nil, err
	}
	config := client3.Config{
		Endpoints:   []string{conn.EtcdEndpoints},
		DialTimeout: clientTimeout,
		TLS:         tlsConfig,
	}
	return client3.New(config)
}
