package net

import (
	"testing"
)

func TestInetIP2Int(t *testing.T) {

	ip2Int := InetIP2Int("10.244.0.0")

	if ip2Int != 183762944 {
		t.Errorf("InetIP2Int(\"10.244.0.0\") = %d; want -2", ip2Int)
	}

	mask := InetIP2Int("255.255.255.0")
	if mask != 4294967040 {
		t.Errorf("InetIP2Int(\"255.255.255.0\") = %d; want -2", mask)
	}

	if ip2Int&mask != 183762944 {
		t.Errorf("InetIP2Int(\"255.255.255.0\") = %d; want -2", mask)
	}
}
