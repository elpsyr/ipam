package net

import (
	"fmt"
	"math/big"
	"net"
)

func GetMaskIpFromNum(numStr string) string {
	switch numStr {
	case "8":
		return "255.0.0.0"
	case "16":
		return "255.255.0.0"
	case "24":
		return "255.255.255.0"
	case "32":
		return "255.255.255.255"
	default:
		return "255.255.0.0"
	}
}

// InetInt2Ip convert ip from int to string
func InetInt2Ip(ip int64) string {
	return fmt.Sprintf("%d.%d.%d.%d",
		byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip))
}

// InetIP2Int convert ip from string to int
func InetIP2Int(ip string) int64 {
	ret := big.NewInt(0)
	ret.SetBytes(net.ParseIP(ip).To4())
	return ret.Int64()
}
