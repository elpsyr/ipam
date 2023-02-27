# ipam
IP Address Management

# ipam 需要做什么
1. 向 etcd 写入 ip池，ip池是所有可以使用的网段ip的合集，如果初始化过了就不用了
2. etcd 中存 当前节点下所有已用ip的集合
3. 记录当前节点分配到的ip网段

# ipam 提供了哪些服务
1. 获取一个当前节点下的未使用ip
2. 获取当前节点的 ip 网关
3. 获取集群所有节点的网络信息
