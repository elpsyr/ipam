package etcd

import (
	client3 "go.etcd.io/etcd/client/v3"
)

// we use  "go.etcd.io/etcd/client/v3" to create a client to operating etcd

func GetClient() (*client3.Client, error) {
	config := client3.Config{
		Endpoints:            nil,
		AutoSyncInterval:     0,
		DialTimeout:          0,
		DialKeepAliveTime:    0,
		DialKeepAliveTimeout: 0,
		MaxCallSendMsgSize:   0,
		MaxCallRecvMsgSize:   0,
		TLS:                  nil,
		Username:             "",
		Password:             "",
		RejectOldCluster:     false,
		DialOptions:          nil,
		Context:              nil,
		Logger:               nil,
		LogConfig:            nil,
		PermitWithoutStream:  false,
	}
	return client3.New(config)
}
