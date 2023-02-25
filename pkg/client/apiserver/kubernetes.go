package apiserver

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func GetClient(configPath string) (*kubernetes.Clientset, error) {
	// 创建 Kubernetes 配置对象
	config, err := rest.InClusterConfig()
	if err != nil {
		// 如果在集群外运行，则从 kubeconfig 文件中读取配置
		config, err = clientcmd.BuildConfigFromFlags("", configPath)
		if err != nil {
			panic(err.Error())
		}
	}

	// 创建 Kubernetes 客户端
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return clientset, nil
}
