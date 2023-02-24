package etcd

import (
	"context"
	"fmt"
	etcd3 "go.etcd.io/etcd/client/v3"
	"testing"
)

func TestGetClient(t *testing.T) {

	_, err := GetClient(&Connection{
		EtcdEndpoints:  "https://172.16.0.124:2379",
		EtcdCertFile:   "D:\\Project\\elpsyr\\ipam\\test\\tls\\healthcheck-client.crt",
		EtcdKeyFile:    "D:\\Project\\elpsyr\\ipam\\test\\tls\\healthcheck-client.key",
		EtcdCACertFile: "D:\\Project\\elpsyr\\ipam\\test\\tls\\ca.crt",
	})
	if err != nil {
		t.Error(err)
	}
}

// ETCDCTL_API=3 etcdctl --endpoints https://172.16.0.124:2379 --cacert /etc/kubernetes/pki/etcd/ca.crt --cert /etc/kubernetes/pki/etcd/healthcheck-client.crt --key /etc/kubernetes/pki/etcd/healthcheck-client.key get / --prefix --keys-only
func TestGet(t *testing.T) {

	client, err := GetClient(&Connection{
		EtcdEndpoints:  "https://172.16.0.124:2379",
		EtcdCertFile:   "D:\\Project\\elpsyr\\ipam\\test\\tls\\healthcheck-client.crt",
		EtcdKeyFile:    "D:\\Project\\elpsyr\\ipam\\test\\tls\\healthcheck-client.key",
		EtcdCACertFile: "D:\\Project\\elpsyr\\ipam\\test\\tls\\ca.crt",
	})
	if err != nil {
		t.Error(err)
	}
	get, err := client.Get(context.TODO(), "/", etcd3.WithPrefix())
	if err != nil {
		t.Error(err)
	}
	for _, v := range get.Kvs {
		fmt.Println(v.String())

	}

}
