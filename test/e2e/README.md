# 使用说明

```shell
[root@localhost e2e]# bash e2e.sh -h
USAGE:
  prepare-kubeconfig  [clusters_kubeconfig_store_dir] [cluster_ip_list_file_path]
                      e.g. prepare-kubeconfigs /tmp/e2ekubeconfigs ./cluster-master-ips
        
                      [clusters_kubeconfig_store_dir] [fabdns_zone|timeout]
                      e.g. /tmp/e2ekubeconfigs
```

## <span id="j1">1.准备多集群kubeconfig文件</span>
**此脚本和fabdns-e2e.test程序可以放在任意集群master节点上执行**
```shell
bash e2e.sh prepare-kubeconfig /tmp/e2ekubeconfigs ./cluster-master-ips
```
>脚本读取./cluster-master-ips文件中各集群master IP列表(文件中每行一个IP)，以scp方式获取集群master节点下/root/.kube/config文件，以对应master IP命名存入目录，如：scp root@10.20.8.20:/root/.kube/config /tmp/e2ekubeconfigs/10.20.8.20
>
>此选项仅为多集群测试准备kubeconfig文件，**可以手动收集各集群kubeconfig文件到一个临时目录**
- 选项：prepare-kubeconfig
- 集群kubeconfig文件目录：/tmp/e2ekubeconfigs (保存至少两个集群主节点配置文件的**临时目录**)
- 集群的主节点IP列表文件：./cluster-master-ips (保存至少两个集群主节点IP地址的文本文件)

    ```shell
    # 集群主节点IP地址独立成行写入文件
    [root@localhost e2e]# cat ./cluster-master-ips 
    10.20.8.20
    10.20.8.4
    10.20.8.12

    # 以对应集群master节点IP命名的kubeconfig文件保存到临时目录
    [root@localhost e2e]# ls -hl /tmp/e2ekubeconfigs/
    -rw-------. 1 root root 5.6K Nov 29 00:38 10.20.8.4
    -rw-------. 1 root root 5.5K Nov 29 00:38 10.20.8.12
    -rw-------. 1 root root 5.5K Nov 29 00:38 10.20.8.20
    ```

## 2.多集群e2e测试
**此脚本和fabdns-e2e.test程序可以放在任意集群master节点上执行**

默认fabdns解析域为global，默认超时时间为300秒，可省略：
```shell
bash e2e.sh /tmp/e2ekubeconfigs
```
- 可以修改解析域名，如：bash e2e.sh /tmp/e2ekubeconfigs **other-zone**
- 可以修改超时时间，如：bash e2e.sh /tmp/e2ekubeconfigs **200**
- 同时修改解析域名和超时时间，如：bash e2e.sh /tmp/e2ekubeconfigs **other-zone** **200**
- 集群kubeconfig文件目录：/tmp/e2ekubeconfigs (保存至少两个集群主节点配置文件的**临时目录**，可以参考[准备多集群kubeconfig文件](#j1)再运行多集群测试)
