# Fabdns配置参考


## 1. fabdns configmap
```yaml
fabdns global {
   masterurl https://10.20.8.24:6443
   kubeconfig /root/.kube/config
   cluster fabedge
   cluster-zone beijing
   cluster-region north
   ttl 30
}
```
- masterurl: 集群API请求URL (集群内不需指定)
- kubeconfig: 集群kubeconfig文件路径 (集群内不需指定)
- cluster: 集群名称
- cluster-zone: 集群所在zone
- cluster-region: 集群所在region
- ttl: DNS TTL (范围(0, 3600]，默认5s)

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
           cluster-zone beijing
           cluster-region north
           ttl 30
        }
        forward . /etc/resolv.conf {
           max_concurrent 1000
        }
        cache 30
    }
```


## 2.coredns configmap
配置文件Corefile设置转发global域的解析到fabdns:
```yaml
 global {
     forward . 10.96.140.51
 }
```
10.96.140.51 即 fabdns service IP **需要根据创建的fabdns service实际ClusterIP进行修改**

样例：
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
**注：coredns原配置保持不变，仅增加转发到fabdns的配置项**