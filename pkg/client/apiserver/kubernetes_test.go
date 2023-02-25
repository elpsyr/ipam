package apiserver

import (
	"context"
	"fmt"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestGetClient(t *testing.T) {
	client, err := GetClient("D:\\Project\\elpsyr\\ipam\\test\\kube\\config")
	if err != nil {
		t.Error(err)

	}
	namespaces, err := client.CoreV1().Namespaces().List(context.TODO(), v1.ListOptions{})
	if err != nil {
		t.Error(err)
	}
	fmt.Println(namespaces)

}
