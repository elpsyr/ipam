# ipam
IP Address Management

# ipam 需要做什么
1. 向 etcd 写入 ip池，ip池是所有可以使用的网段ip的合集，如果初始化过了就不用了
2. etcd 中存 当前节点下所有已用ip的集合
3. 记录当前节点分配到的ip网段
