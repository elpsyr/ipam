package ipam

import "testing"

// 目前测试数据需要手动删除：
// ETCDCTL_API=3 etcdctl --endpoints https://172.16.0.124:2379 --cacert /etc/kubernetes/pki/etcd/ca.crt --cert /etc/kubernetes/pki/etcd/healthcheck-client.crt --key /etc/kubernetes/pki/etcd/healthcheck-client.key del  /testcni/ipam --prefix
func TestNew(t *testing.T) {
	_, err := New(Config{
		Subnet: "10.244.0.0/16",
		conn: ConnectionInfo{
			EtcdEndpoints:  "https://172.16.0.124:2379",
			EtcdCertFile:   "D:\\Project\\elpsyr\\ipam\\test\\tls\\healthcheck-client.crt",
			EtcdKeyFile:    "D:\\Project\\elpsyr\\ipam\\test\\tls\\healthcheck-client.key",
			EtcdCACertFile: "D:\\Project\\elpsyr\\ipam\\test\\tls\\ca.crt",
		},
	})
	if err != nil {
		t.Error(err)
	}
}
