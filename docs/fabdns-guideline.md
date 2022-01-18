# Fabdns使用指南

## 1. fabdns 参数说明
```yaml
fabdns global {
   masterurl https://10.20.8.24:6443
   kubeconfig /root/.kube/config
   cluster fabedge
   zone beijing
   region north
   ttl 30
}
```
- masterurl: 集群API请求URL (集群内不需指定)
- kubeconfig: 集群kubeconfig文件路径 (集群内不需指定)
- cluster: 集群名称
- zone: 集群所在zone
- region: 集群所在region
- ttl: DNS TTL (范围[0, 3600]，默认5s)

样例：
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: fabdns
  namespace: fabedge
data:
  Corefile: |
    .:53 {
        errors
        health
        ready
        fabdns global {
           cluster fabedge
           zone beijing
           region north
           ttl 30
        }
        cache 30
        reload
    }
```


## 2.coredns 转发配置
要解析global域的域名，除了启动fabdns外，还需要在coredns里增加转发配置项: 
```yaml
 global {
     forward . 10.96.140.51
 }
```
10.96.140.51是 fabdns service的ClusterIP，根据实际实际情况配置**

有些环境的DNS解析服务可能不是直接由kube-system/coredns来负责，而是由其他组件比如nodelocaldns,edge-coredns解析，这时候需要修改这些组件的配置，下面以kube-system/coredns configmap为例：

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: coredns
  namespace: kube-system
data:
  Corefile: |
    global {
        forward . 10.96.140.51
    }
    
    .:53 {
        errors
        health {
           lameduck 5s
        }
        ready
        kubernetes cluster.local in-addr.arpa ip6.arpa {
           pods insecure
           fallthrough in-addr.arpa ip6.arpa
           ttl 30
        }
        prometheus :9153
        forward . /etc/resolv.conf {
           max_concurrent 1000
        }
        cache 30
        loop
        reload
        loadbalance
    }
```
