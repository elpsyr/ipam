package ipam

import (
	"fmt"
	"os"
	"sync"
	"testing"
)

// windows connectionInfoWindows to 172.16.0.124  kube & etcd
var connectionInfoWindows = ConnectionInfo{
	EtcdEndpoints:  "https://172.16.0.124:2379",
	EtcdCertFile:   "D:\\Project\\elpsyr\\ipam\\test\\tls\\healthcheck-client.crt",
	EtcdKeyFile:    "D:\\Project\\elpsyr\\ipam\\test\\tls\\healthcheck-client.key",
	EtcdCACertFile: "D:\\Project\\elpsyr\\ipam\\test\\tls\\ca.crt",
	KubeConfigPath: "D:\\Project\\elpsyr\\ipam\\test\\kube\\config",
}

const (
	linuxProjectBase   = "/mnt/d/Project/elpsyr/ipam"
	windowsProjectBase = "/mnt/d/Project/elpsyr/ipam"
)

// linux connectionInfoWindows to 172.16.0.124  kube & etcd
var connectionInfoLinux = ConnectionInfo{
	EtcdEndpoints:  "https://172.16.0.124:2379",
	EtcdCertFile:   linuxProjectBase + "/test/tls/healthcheck-client.crt",
	EtcdKeyFile:    linuxProjectBase + "/test/tls/healthcheck-client.key",
	EtcdCACertFile: linuxProjectBase + "/test/tls/ca.crt",
	KubeConfigPath: linuxProjectBase + "/test/kube/config",
}

// 目前测试数据需要手动删除：
// ETCDCTL_API=3 etcdctl --endpoints https://172.16.0.124:2379 --cacert /etc/kubernetes/pki/etcd/ca.crt --cert /etc/kubernetes/pki/etcd/healthcheck-client.crt --key /etc/kubernetes/pki/etcd/healthcheck-client.key del  /testcni/ipam --prefix
func TestNew(t *testing.T) {
	_, err := New(Config{
		Subnet: "10.244.0.0/16",
		Conn:   connectionInfoWindows,
	})
	if err != nil {
		t.Error(err)
	}
}

// 测试多个客户端同时从 subnet IP池获取一个 subnet IP
func TestConcurrencyGetIP(t *testing.T) {

	// 前置条件 创建IP池
	ipam, err := New(Config{
		Subnet: "10.244.0.0/16",
		Conn:   connectionInfoWindows,
	})
	if err != nil {
		t.Error(err)
	}

	var wg sync.WaitGroup
	// mock 多客户端获取IP
	for i := 0; i < 10; i++ {
		wg.Add(1)
		poolPath := getEtcdPathWithPrefix("/" + ipam.Subnet + "/" + ipam.MaskSegment + "/" + "pool")
		go func() {
			defer wg.Done()
			ip, _ := ipam.getSubnetIpFromPools(poolPath)
			fmt.Println(ip)
		}()
	}
	wg.Wait()
}

// 测试单个客户端获取 subnet 下ip地址
func TestIpAddressManagement_GetUnusedIP(t *testing.T) {

	// 前置条件 创建IP池
	ipam, err := New(Config{
		Subnet: "10.244.0.0/16",
		Conn:   connectionInfoWindows,
	})
	if err != nil {
		t.Error(err)
	}

	var wg sync.WaitGroup
	// mock 单个客户端多次获取 IP
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unusedIP, _ := ipam.GetUnusedIP()
			fmt.Println(unusedIP)
		}()
	}
	wg.Wait()
}

func TestAllHostNetwork(t *testing.T) {
	// 前置条件 mock两个节点
	ipam, err := NewWithOptions(Config{
		Subnet: "10.244.0.0/16",
		Conn:   connectionInfoWindows,
	}, &InitOptions{HostName: "172-16-0-130"})
	if err != nil {
		t.Error(err)
	}
	// reset
	im = nil
	iPamImplement = nil
	//
	ipam, err = NewWithOptions(Config{
		Subnet: "10.244.0.0/16",
		Conn:   connectionInfoWindows,
	}, &InitOptions{HostName: "172-16-0-124"})
	if err != nil {
		t.Error(err)
	}

	allHostNetwork, err := ipam.AllHostNetwork()
	if err != nil {
		t.Error(err)
	}
	for i, network := range allHostNetwork {

		fmt.Printf("the %d  : %v ", i, network)
	}
}

func TestHostNetwork(t *testing.T) {
	// 前置条件 创建IP池
	ipam, err := New(Config{
		Subnet: "10.244.0.0/16",
		Conn:   connectionInfoLinux,
	})
	if err != nil {
		t.Error(err)
	}

	// mock hostname
	hostname, err := os.Hostname()
	if err != nil {
		t.Error(err)
	}
	hostNetwork, err := ipam.HostNetwork(hostname)

	if err != nil {
		t.Error(err)
	}
	fmt.Println(hostNetwork)
}
