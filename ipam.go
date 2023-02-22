package ipam

import (
	"errors"
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
	//EtcdClient         *etcd.EtcdClient
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

func IpAddressManagementV1(subnet string, options *InitOptions) func() (*IpAddressManagement, error) {

	return func() (*IpAddressManagement, error) {
		return nil, nil
	}
}
