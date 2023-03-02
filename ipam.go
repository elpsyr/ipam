package ipam

import (
	"context"
	"errors"
	"fmt"
	"github.com/elpsyr/ipam/consts"
	"github.com/elpsyr/ipam/pkg/client/apiserver"
	"github.com/elpsyr/ipam/pkg/client/etcd"
	"github.com/elpsyr/ipam/pkg/net"
	"github.com/vishvananda/netlink"
	etcd3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	prefix = "testcni/ipam"
)

var iPamImplement func() (*IpAddressManagement, error)

var im *IpAddressManagement

var _lock sync.Mutex

// IpAddressManagement support ip address management
type IpAddressManagement struct {
	Subnet             string
	MaskSegment        string
	MaskIP             string
	PodMaskSegment     string
	PodMaskIP          string
	CurrentHostNetwork string
	EtcdClient         *etcd3.Client
	K8sClient          *kubernetes.Clientset
	*Cache
	//*operator
}
type Cache struct {
	nodeIpCache map[string]string
	cidrCache   map[string]string
}

// InitOptions store information to init IpAddressManagement
type InitOptions struct {
	MaskSegment      string
	PodIpMaskSegment string
	RangeStart       string
	RangeEnd         string
	HostName         string // test used
}

// Config contains information about starting ip_address_management
type Config struct {
	Subnet string
	Conn   ConnectionInfo // Connection info of etcd & kubernetes
}

// ConnectionInfo is Connection info of etcd & kubernetes
type ConnectionInfo struct {
	EtcdEndpoints  string // https://192.168.98.143:2379
	EtcdCertFile   string // /etc/kubernetes/pki/etcd/healthcheck-client.crt
	EtcdKeyFile    string // /etc/kubernetes/pki/etcd/healthcheck-client.key
	EtcdCACertFile string // /etc/kubernetes/pki/etcd/ca.crt
	KubeConfigPath string
}

func New(cfg Config) (*IpAddressManagement, error) {
	if err := Init(cfg.Subnet, nil, &cfg.Conn); err != nil {
		return nil, err
	}
	return getInitializedIpAddressManagement()
}

func NewWithOptions(cfg Config, opt *InitOptions) (*IpAddressManagement, error) {
	if err := Init(cfg.Subnet, opt, &cfg.Conn); err != nil {
		return nil, err
	}
	return getInitializedIpAddressManagement()
}

// Gateway return the first ip of this subnet
// if subnet is 10.244.0.0 , return 10.244.0.1 as gateway
func (is *IpAddressManagement) Gateway() (string, error) {

	resp, err := is.EtcdClient.Get(context.TODO(), getHostPath())
	currentNetwork := string(resp.Kvs[0].Value)
	if err != nil {
		return "", err
	}

	return net.InetInt2Ip(net.InetIP2Int(currentNetwork) + 1), nil
}

func (is *IpAddressManagement) GatewayWithMaskSegment() (string, error) {
	resp, err := is.EtcdClient.Get(context.TODO(), getHostPath())
	currentNetwork := string(resp.Kvs[0].Value)
	if err != nil {
		return "", err
	}

	return net.InetInt2Ip(net.InetIP2Int(currentNetwork)+1) + "/" + getIpamMaskSegment(), nil

}

// GetUnusedIP 获取一个当前节点所属子网下的 pod ip地址 范围2-255
// 注意：该函数内部需要加锁，防止同一时刻不同pod获取到了相同的ip
func (is *IpAddressManagement) GetUnusedIP() (string, error) {
	_lock.Lock()
	defer _lock.Unlock()
	for {
		ip, err := is.nextUnusedIP()
		if err != nil {
			return "", err
		}
		if net.IsGatewayIP(ip) || net.IsRetainIP(ip) {
			err = is.recordIP(ip)
			if err != nil {
				return "", err
			}
			continue
		}
		// 先把这个 ip 占上坑位
		// 坑位先占上不影响大局
		// 但是如果坑位占晚了被别人抢先的话可能会导致有俩 pod 的 ip 冲突
		err = is.recordIP(ip)
		if err != nil {
			return "", err
		}
		return ip, nil
	}
}

func (is *IpAddressManagement) nextUnusedIP() (string, error) {
	// 获取当前节点所属的网段

	currentNodeSubnetNetwork, err := is.EtcdClient.Get(context.TODO(), getHostPath())
	if err != nil {
		return "", err
	}
	allUsedIPsResp, err := is.EtcdClient.Get(context.TODO(), getRecordPath(string(currentNodeSubnetNetwork.Kvs[0].Value)))
	if err != nil {
		return "", err
	}
	var allUsedIPs string
	if len(allUsedIPsResp.Kvs) > 0 {
		allUsedIPs = string(allUsedIPsResp.Kvs[0].Value)
	} else {
		allUsedIPs = ""
	}
	ipsMap := map[string]bool{}
	ips := strings.Split(allUsedIPs, ";")

	// 标记该IP已经使用
	for _, ip := range ips {
		ipsMap[ip] = true
	}

	nextIp := ""
	start := net.InetIP2Int(string(currentNodeSubnetNetwork.Kvs[0].Value))
	for i := 0; i < 256; i++ {
		nextIpNum := start + int64(i)
		nextIp = net.InetInt2Ip(nextIpNum)
		if _, ok := ipsMap[nextIp]; !ok {
			break
		}
	}

	return nextIp, nil
}

func (is *IpAddressManagement) recordIP(ip string) error {

	// 获取当前节点所属的网段

	currentNodeSubnetNetwork, err := is.EtcdClient.Get(context.TODO(), getHostPath())
	if err != nil {
		return err
	}
	allUsedIPsResp, err := is.EtcdClient.Get(context.TODO(), getRecordPath(string(currentNodeSubnetNetwork.Kvs[0].Value)))
	if err != nil {
		return err
	}
	var allUsedIPs string
	if len(allUsedIPsResp.Kvs) > 0 {
		allUsedIPs = string(allUsedIPsResp.Kvs[0].Value)
	} else {
		allUsedIPs = ""
	}
	ipsMap := map[string]bool{}
	ips := strings.Split(allUsedIPs, ";")

	// 标记该IP已经使用
	for _, ip := range ips {
		ipsMap[ip] = true
	}

	nextIp := ip

	if _, ok := ipsMap[nextIp]; ok {
		return errors.New("already record")
	}
	if allUsedIPs != "" {
		ips = append(ips, nextIp)
	} else {
		ips = []string{nextIp}
	}
	joinedIPs := strings.Join(ips, ";")
	_, err = is.EtcdClient.Put(context.TODO(), getRecordPath(string(currentNodeSubnetNetwork.Kvs[0].Value)), joinedIPs)
	if err != nil {
		return err
	}

	return nil
}

func getHostPath() string {
	_im, err := getInitializedIpAddressManagement()
	if err != nil {
		return "/test-error-path"
	}
	hostname, err := os.Hostname()
	if err != nil {
		return "/test-error-path"
	}
	return getEtcdPathWithPrefix("/" + _im.Subnet + "/" + _im.MaskSegment + "/" + hostname)
}

func getRecordPath(hostNetwork string) string {
	return getHostPath() + "/" + hostNetwork
}

func getIpamMaskSegment() string {
	_im, err := getInitializedIpAddressManagement()
	if err != nil {
		return "/test-error-mask"
	}
	return _im.MaskSegment
}

func Init(subnet string, options *InitOptions, conn *ConnectionInfo) error {
	if iPamImplement == nil {
		// now we use IpAddressManagementV1 to implement
		iPamImplement = IpAddressManagementV1(subnet, options, conn)
	}
	_, err := getInitializedIpAddressManagement()
	return err
}

func getInitializedIpAddressManagement() (*IpAddressManagement, error) {

	if iPamImplement == nil {
		return nil, errors.New("iPamImplement should be assigned first")
	}
	return iPamImplement()
}

// IpAddressManagementV1 subnet like 10.244.0.0/16
func IpAddressManagementV1(subnet string, options *InitOptions, conn *ConnectionInfo) func() (*IpAddressManagement, error) {

	return func() (*IpAddressManagement, error) {
		if im != nil {
			return im, nil
		} else {
			// 1. get a IpAddressManagement instance
			// set default
			_subnet := subnet
			var _maskSegment string = consts.DefaultMaskNum      // 24
			var _podIpMaskSegment string = consts.DefaultMaskNum // 24
			var _rangeStart string = ""
			var _rangeEnd string = ""
			hostname, err := os.Hostname()
			cache := &Cache{
				cidrCache:   map[string]string{},
				nodeIpCache: map[string]string{},
			}
			// set options
			if options != nil {
				if options.MaskSegment != "" {
					_maskSegment = options.MaskSegment
				}
				if options.PodIpMaskSegment != "" {
					_podIpMaskSegment = options.PodIpMaskSegment
				}
				if options.RangeStart != "" {
					_rangeStart = options.RangeStart
				}
				if options.RangeEnd != "" {
					_rangeEnd = options.RangeEnd
				}
				if options.HostName != "" {
					hostname = options.HostName
				}
			}

			// 配置文件中传参数的时候可能直接传了个子网掩码
			// 传了的话就直接使用这个掩码
			if withMask := strings.Contains(subnet, "/"); withMask {
				subnetAndMask := strings.Split(subnet, "/")
				_subnet = subnetAndMask[0]
				_maskSegment = subnetAndMask[1]
			}

			var _maskIP string = net.GetMaskIpFromNum(_maskSegment)
			var _podMaskIP string = net.GetMaskIpFromNum(_podIpMaskSegment)

			// 如果不是合法的子网地址的话就强转成合法
			// 比如 _subnet 传了个数字过来, 要给它先干成 a.b.c.d 的样子
			// 然后 & maskIP, 给做成类似 a.b.0.0 的样子
			_subnet = net.InetInt2Ip(net.InetIP2Int(_subnet) & net.InetIP2Int(_maskIP))
			im = &IpAddressManagement{
				Subnet:         _subnet,           // 子网网段
				MaskSegment:    _maskSegment,      // 掩码 10 进制
				MaskIP:         _maskIP,           // 掩码 ip
				PodMaskSegment: _podIpMaskSegment, // pod 的 mask 10 进制
				PodMaskIP:      _podMaskIP,        // pod 的 mask ip
				Cache:          cache,
			}
			etcdClient, err := etcd.GetClient(&etcd.Connection{
				EtcdEndpoints:  conn.EtcdEndpoints,
				EtcdCertFile:   conn.EtcdCertFile,
				EtcdKeyFile:    conn.EtcdKeyFile,
				EtcdCACertFile: conn.EtcdCACertFile,
			})
			im.EtcdClient = etcdClient
			client, err := apiserver.GetClient(conn.KubeConfigPath)
			im.K8sClient = client

			// 2. init ip pool
			// 初始化一个 ip 网段的 pool
			// 如果已经初始化过就不再初始化
			poolPath := getEtcdPathWithPrefix("/" + im.Subnet + "/" + im.MaskSegment + "/" + "pool")
			err = im.ipsPoolInit(poolPath)
			if err != nil {
				return nil, err
			}

			// 3. get a subnet for local
			// 然后尝试去拿一个当前主机可用的网段
			// 如果拿不到, 里面会尝试创建一个
			//hostname, err := os.Hostname()
			if err != nil {
				return nil, err
			}
			// k==>v
			// hostPath==>ip
			hostPath := getEtcdPathWithPrefix("/" + im.Subnet + "/" + im.MaskSegment + "/" + hostname)
			currentHostNetwork, err := im.localNetworkInit(
				hostPath, // local
				poolPath, // ip set
				_rangeStart,
				_rangeEnd,
			)
			if err != nil {
				return nil, err
			}

			// 初始化一个 map 的地址给 ebpf 用
			//err = _ipam.subnetMapInit(
			//	_subnet,
			//	_maskSegment,
			//	hostname,
			//	currentHostNetwork,
			//)
			if err != nil {
				return nil, err
			}

			im.CurrentHostNetwork = currentHostNetwork
			return im, nil
		}
	}
}

// getEtcdPathWithPrefix add prefix to this path
func getEtcdPathWithPrefix(path string) string {
	// if path start with "/" ,like  "/foo"
	if path != "" && path[0:1] == "/" {
		return "/" + prefix + path
	}
	// or path start without "/" ,like  "foo"
	return "/" + prefix + "/" + path
}

// ipsPoolInit init a ip pool like:
// 比如 subnet 是 10.244.0.0, mask 是 24 的话
// 就会在 etcd 中初始化出一个
// 10.244.0.0;10.244.1.0;10.244.2.0;......;10.244.254.0;10.244.255.0
func (is *IpAddressManagement) ipsPoolInit(poolPath string) error {

	resp, err := is.EtcdClient.Get(context.TODO(), poolPath)
	if err != nil {
		return err
	}
	if len(resp.Kvs) > 0 {
		if len(string(resp.Kvs[len(resp.Kvs)-1:][0].Value)) > 0 {
			return nil
		}
	}

	subnet := is.Subnet
	_temp := strings.Split(subnet, ".")
	_tempIndex := 0
	for _i := 0; _i < len(_temp); _i++ {
		if _temp[_i] == "0" {
			// 找到 subnet 中第一个 0 的位置
			_tempIndex = _i
			break
		}
	}
	/**
	 * FIXME: 对于子网网段的创建, 其实可以不完全是 8 的倍数
	 * 比如 10.244.0.0/26 这种其实也可以
	 */
	// 创建出 255 个备用的网段
	// 每个节点从这些网段中选择一个还没有使用过的
	_tempIpStr := ""
	for _j := 0; _j <= 255; _j++ {
		_temp[_tempIndex] = fmt.Sprintf("%d", _j)
		_newIP := strings.Join(_temp, ".")
		if _tempIpStr == "" {
			_tempIpStr = _newIP
		} else {
			_tempIpStr += ";" + _newIP
		}
	}
	_, err = is.EtcdClient.Put(context.TODO(), poolPath, _tempIpStr)
	return err
}

// localNetworkInit get a subnet(ip) for local node
// 为当前主机获取一个可用的网段,并且记录在而 etcd 中
func (is *IpAddressManagement) localNetworkInit(hostPath, poolPath string, ranges ...string) (string, error) {

	resp, err := is.EtcdClient.Get(context.TODO(), hostPath, etcd3.WithSort(etcd3.SortByVersion, etcd3.SortDescend), etcd3.WithLimit(1))
	if err != nil {
		return "", err
	}
	for _, ev := range resp.Kvs {
		network := string(ev.Value)
		// 已经存过该主机对应的网段了
		if network != "" {
			return network, nil
		}
	}

	// 从可用的 ip 池中捞一个
	currentHostNetwork, err := is.getSubnetIpFromPools(poolPath)
	if err != nil {
		return "", err
	}
	// 再把这个网段存到对应的这台主机的 key 下
	_, err = is.EtcdClient.Put(context.TODO(), hostPath, currentHostNetwork)
	if err != nil {
		return "", err
	}

	// Todo: ip address range
	return currentHostNetwork, nil
}

func (is *IpAddressManagement) getSubnetIpFromPools(poolPath string) (string, error) {
	var lockRetryDelay = 500 * time.Millisecond
	var currentHostNetwork string
	// 创建一个Session
	session, err := concurrency.NewSession(is.EtcdClient)
	if err != nil {
		return "err/get/session", err
	}
	defer session.Close()

	// 创建一个基于lease的分布式锁
	mutex := concurrency.NewMutex(session, "/lock")
	for {
		// 尝试获取分布式锁
		err = mutex.Lock(context.Background())
		if err != nil {
			//fmt.Printf("Failed to acquire lock: %v\n", err)
			time.Sleep(lockRetryDelay)
			continue
		}

		// 获取锁成功，读取和修改 key 的值
		resp, err := is.EtcdClient.Get(context.Background(), poolPath)
		if err != nil {
			return "err/get/poolPath", err
			//fmt.Printf("Failed to get value: %v\n", err)
		} else {
			value := string(resp.Kvs[0].Value)
			//fmt.Printf("Current value: %s\n", value)

			// 解析字符串并修改数字
			ips := strings.Split(string(value), ";")
			currentHostNetwork = ips[0]
			newIpPools := strings.Join(ips[1:], ";")

			// 将更新后的字符串存储回etcd
			_, err = is.EtcdClient.Put(context.Background(), poolPath, newIpPools)
			if err != nil {
				//fmt.Printf("Failed to update value: %v\n", err)
			} else {
				//fmt.Printf("Updated value: %s\n", value)
			}
		}

		// 释放分布式锁
		err = mutex.Unlock(context.Background())
		if err != nil {
			return "err/release/lock", err
			//fmt.Printf("Failed to release lock: %v\n", err)
		}
		break
		// 等待一段时间后重试
		//time.Sleep(lockRetryDelay)
	}
	return currentHostNetwork, nil
}

type Network struct {
	Name          string
	IP            string
	Hostname      string
	CIDR          string
	IsCurrentHost bool
}

func (is *IpAddressManagement) HostNetwork(hostname string) (*Network, error) {
	linkList, err := netlink.LinkList()
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}

	hostIP, err := is.NodeIp(hostname)
	if err != nil {
		return nil, err
	}
	for _, link := range linkList {
		// 就看类型是 device 的
		if link.Type() == "device" {
			// 找每块儿设备的地址信息
			addr, err := netlink.AddrList(link, netlink.FAMILY_V4)
			if err != nil {
				continue
			}
			if len(addr) >= 1 {
				// TODO: 这里其实应该处理一下一块儿网卡绑定了多个 ip 的情况
				// 数组中的每项都是这样的格式 "192.168.98.143/24 ens33"
				_addr := strings.Split(addr[0].String(), " ")
				ip := _addr[0]
				name := _addr[1]
				ip = strings.Split(ip, "/")[0]
				if ip == hostIP {
					// 走到这儿说明主机走的就是这块儿网卡
					return &Network{
						Name:          name,
						IP:            hostIP,
						Hostname:      hostname,
						IsCurrentHost: true,
					}, nil
				}
			}
		}
	}
	return nil, errors.New("no valid network device found")
}

// AllHostNetwork 获取集群中全部节点的网络信息
func (is *IpAddressManagement) AllHostNetwork() ([]*Network, error) {
	names, err := is.NodeNames()
	if err != nil {
		return nil, err
	}

	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}

	res := []*Network{}
	for _, name := range names {
		ip, err := is.NodeIp(name)
		if err != nil {
			return nil, err
		}

		cidr, err := is.CIDR(name)
		if err != nil {
			return nil, err
		}

		if name == hostname {
			res = append(res, &Network{
				Hostname:      name,
				IP:            ip,
				IsCurrentHost: true,
				CIDR:          cidr,
			})
		} else {
			res = append(res, &Network{
				Hostname:      name,
				IP:            ip,
				IsCurrentHost: false,
				CIDR:          cidr,
			})
		}
	}
	return res, nil
}

// NodeNames get all cluster nodes hostname
// 用来获取集群中全部的 host name
// 这里直接从 etcd 的 key 下边查
// 不调 k8s 去捞, k8s 捞一次出来的东西太多了
func (is *IpAddressManagement) NodeNames() ([]string, error) {
	const _minionsNodePrefix = "/registry/minions/"

	// 1. get all nodes hostname
	// like: /registry/minions/172-16-0-124
	var nodes []string
	nodesResp, err := is.EtcdClient.Get(context.TODO(), _minionsNodePrefix, etcd3.WithKeysOnly(), etcd3.WithPrefix())
	for _, ev := range nodesResp.Kvs {
		nodes = append(nodes, string(ev.Key))
	}

	if err != nil {
		return nil, err
	}

	// 2. remove prefix
	var res []string
	for _, node := range nodes {
		node = strings.Replace(node, _minionsNodePrefix, "", 1)
		res = append(res, node)
	}
	return res, nil
}

/**
 * 根据 host name 获取节点 ip
 */
func (is *IpAddressManagement) NodeIp(hostName string) (string, error) {
	if val, ok := is.nodeIpCache[hostName]; ok {
		return val, nil
	}
	node, err := is.K8sClient.CoreV1().Nodes().Get(context.TODO(), hostName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	for _, addr := range node.Status.Addresses {
		if addr.Type == "InternalIP" {
			is.nodeIpCache[hostName] = addr.Address
			return addr.Address, nil
		}
	}
	return "", errors.New("没有找到 ip")
}

// 获取当前节点被分配到的网段 + mask
func (is *IpAddressManagement) CIDR(hostName string) (string, error) {
	if val, ok := is.cidrCache[hostName]; ok {
		return val, nil
	}
	_cidrPath := getEtcdPathWithPrefix("/" + is.Subnet + "/" + getIpamMaskSegment() + "/" + hostName)

	var cidr string
	cidrResp, err := is.EtcdClient.Get(context.TODO(), _cidrPath)
	if err != nil {
		return "", err
	}
	if len(cidrResp.Kvs) > 0 {

		cidr = string(cidrResp.Kvs[0].Value)
	} else {
		cidr = ""
	}

	if cidr == "" {
		return "", nil
	}
	// 先获取一下 ipam
	if err != nil {
		return "", err
	}
	cidr += ("/" + is.PodMaskSegment)
	is.cidrCache[hostName] = cidr
	return cidr, nil
}
