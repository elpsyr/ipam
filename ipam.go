package ipam

import (
	"context"
	"errors"
	"fmt"
	"github.com/elpsyr/ipam/consts"
	"github.com/elpsyr/ipam/pkg/client/etcd"
	"github.com/elpsyr/ipam/pkg/net"
	etcd3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
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
	//K8sClient          *client.LightK8sClient
	//*operator
}

// InitOptions store information to init IpAddressManagement
type InitOptions struct {
	MaskSegment      string
	PodIpMaskSegment string
	RangeStart       string
	RangeEnd         string
}

type Config struct {
	Subnet string
	conn   ConnectionInfo
}

type ConnectionInfo struct {
	EtcdEndpoints  string // https://192.168.98.143:2379
	EtcdCertFile   string // /etc/kubernetes/pki/etcd/healthcheck-client.crt
	EtcdKeyFile    string // /etc/kubernetes/pki/etcd/healthcheck-client.key
	EtcdCACertFile string // /etc/kubernetes/pki/etcd/ca.crt
}

func New(cfg Config) (*IpAddressManagement, error) {
	if err := Init(cfg.Subnet, nil, &cfg.conn); err != nil {
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
			}
			etcdClient, err := etcd.GetClient(&etcd.Connection{
				EtcdEndpoints:  conn.EtcdEndpoints,
				EtcdCertFile:   conn.EtcdCertFile,
				EtcdKeyFile:    conn.EtcdKeyFile,
				EtcdCACertFile: conn.EtcdCACertFile,
			})
			im.EtcdClient = etcdClient
			//_ipam.K8sClient = getLightK8sClient()

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
			hostname, err := os.Hostname()
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
