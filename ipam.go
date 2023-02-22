package ipam

import (
	"context"
	"errors"
	"fmt"
	"github.com/elpsyr/ipam/consts"
	"github.com/elpsyr/ipam/pkg/client/etcd"
	"github.com/elpsyr/ipam/pkg/net"
	etcd3 "go.etcd.io/etcd/client/v3"
	"os"
	"strings"
)

const (
	prefix = "testcni/ipam"
)

var iPamImplement func() (*IpAddressManagement, error)

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

func Init(subnet string, options *InitOptions) error {
	if iPamImplement == nil {
		// now we use IpAddressManagementV1 to implement
		iPamImplement = IpAddressManagementV1(subnet, options)
	}
	_, err := initIpAddressManagement()
	return err
}

func initIpAddressManagement() (*IpAddressManagement, error) {

	if iPamImplement == nil {
		return nil, errors.New("should Init first")
	}
	return iPamImplement()
}

// IpAddressManagementV1 subnet like 10.244.0.0/16
func IpAddressManagementV1(subnet string, options *InitOptions) func() (*IpAddressManagement, error) {

	return func() (*IpAddressManagement, error) {
		var _ipam *IpAddressManagement

		if _ipam != nil {
			return _ipam, nil
		} else {
			_subnet := subnet
			var _maskSegment string = consts.DefaultMaskNum
			var _podIpMaskSegment string = consts.DefaultMaskNum
			var _rangeStart string = ""
			var _rangeEnd string = ""
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
			_ipam = &IpAddressManagement{
				Subnet:         _subnet,           // 子网网段
				MaskSegment:    _maskSegment,      // 掩码 10 进制
				MaskIP:         _maskIP,           // 掩码 ip
				PodMaskSegment: _podIpMaskSegment, // pod 的 mask 10 进制
				PodMaskIP:      _podMaskIP,        // pod 的 mask ip
			}
			etcdClient, err := etcd.GetClient()
			_ipam.EtcdClient = etcdClient
			//_ipam.K8sClient = getLightK8sClient()
			// 初始化一个 ip 网段的 pool
			// 如果已经初始化过就不再初始化
			poolPath := getEtcdPathWithPrefix("/" + _ipam.Subnet + "/" + _ipam.MaskSegment + "/" + "pool")
			err = _ipam.ipsPoolInit(poolPath)
			if err != nil {
				return nil, err
			}

			// 然后尝试去拿一个当前主机可用的网段
			// 如果拿不到, 里面会尝试创建一个
			hostname, err := os.Hostname()
			if err != nil {
				return nil, err
			}
			hostPath := getEtcdPathWithPrefix("/" + _ipam.Subnet + "/" + _ipam.MaskSegment + "/" + hostname)
			currentHostNetwork, err := _ipam.networkInit(
				hostPath,
				poolPath,
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

			_ipam.CurrentHostNetwork = currentHostNetwork
			return _ipam, nil
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

// 根据主机名获取一个当前主机可用的网段
func (is *IpAddressManagement) networkInit(hostPath, poolPath string, ranges ...string) (string, error) {

	return "currentHostNetwork", nil
}
