# 如何创建全局服务

全局服务(GlobalService)是FabDNS提出的一个CRD，通过全局服务域名解析，用户可以实现跨集群的服务访问，更多资料请参考[基于域名解析的多集群服务发现](./multi-cluster-service-discovery-design.md)。本文主要介绍如何创建一个全局服务并如何访问这个服务。

假设存在三个Kubernetes集群，拓扑信息如下:

```markdown
| Name         | Region | Zone     |
|--------------|--------|----------|
| chaoyang     | north  | beijing  |
| pudong       | east   | shanghai |
| shijiazhuang | north  | hebei    |
```



## ClusterIP 服务示例

假设三个集群都在default这个命名空间下有个nginx服务，定义如下:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: nginx
  namespace: default
spec:
  type: ClusterIP
  ports:
  - name: default
    port: 80
    protocol: TCP
    targetPort: 80
  selector:
    app: nginx
```

其中，chaoyang和pudong的集群要把nginx服务暴露出去，组成一个全局服务。执行如下命令暴露nginx服务:

```
kubectl label -n default svc nginx fabedge.io/global-service=true
```

等待FabDNS将数据同步，然后在三个集群就会看到globalservice被创建出来:

```shell
# kubectl get globalservice -n default
NAME            TYPE       AGE
nginx          ClusterIP   5d23h
```

可以查看一下这个全局服务的信息, 可能会看到如下信息:

```
kubectl get globalservice nginx -n default -o yaml
apiVersion: dns.fabedge.io/v1alpha1
kind: GlobalService
metadata:
  labels:
    fabedge.io/created-by: service-hub
    fabedge.io/origin-resource-version: "1014762"
  name: nginx
  namespace: default
spec:
  endpoints:
  - addresses:
    - 10.233.38.92
    cluster: chaoyang
    region: north
    zone: beijing
  - addresses:
    - 10.234.11.20
    cluster: pudong
    region: east
    zone: shanghai
  ports:
  - name: default
    port: 80
    protocol: TCP
  type: ClusterIP

```



这时可以在本集群的任意pod通过域名`nginx.default.svc.global`访问nginx这个全局服务了。FabDNS提供了简单的拓扑感知，如果你在beijing集群访问`nginx.default.svc.global`，这是提供响应的会是beijing集群的nginx服务；如果是在shijiazhuang集群访问这个域名，响应服务的可能是beijing或shanghai的nginx服务。

如果想访问某个集群的nginx服务，比如chaoyang的nginx服务，可以用`chaoyang.nginx.default.svc.global`去访问。但不能访问shijiazhuang的nginx服务，因为它不是全局服务。



## Headless服务

假设三个集群都在default这个命名空间下有个mysql服务，定义如下:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: mysql
  namespace: default
spec:
  type: None
  ports:
  - name: default
    port: 3306
    protocol: TCP
    targetPort: 3306
  selector:
    app: mysql
```

另假设每个mysql服务都有两个endpoint: mysql-0 和 mysql-1。

其中，chaoyang和pudong的集群要把mysql服务暴露出去，组成一个全局服务。等FabDNS同步完数据后，查看mysql globalservice的信息，能看到如下类似数据:

```yaml
kind: GlobalService
metadata:
  name: mysql
  namespace: default
  labels:
    fabedge.io/created-by: service-hub
    fabedge.io/origin-resource-version: "2461075"
spec:
  endpoints:
  - addresses:
    - 10.233.64.39
    cluster: chaoyang
    hostname: mysql-0
    region: north
    targetRef:
      kind: Pod
      name: mysql-0
      namespace: default
      resourceVersion: "369599"
      uid: c707d468-d272-4f46-96e1-c3be6d18abf6
    zone: beijing
  - addresses:
    - 10.233.64.41
    cluster: chaoyang
    hostname: mysql-1
    region: north
    targetRef:
      kind: Pod
      name: mysql-1
      namespace: default
      resourceVersion: "369622"
      uid: d1a597eb-2bfc-42f9-8b2e-e938c2680591
    zone: beijing
  - addresses:
    - 10.234.90.114
    cluster: pudong
    hostname: mysql-0
    region: north
    targetRef:
      kind: Pod
      name: mysql-0
      namespace: default
      resourceVersion: "727100"
      uid: 8f0eccba-e993-4411-90da-63716bb5a737
    zone: shanghai
  - addresses:
    - 10.234.90.116
    cluster: pudong
    hostname: mysql-1
    region: north
    targetRef:
      kind: Pod
      name: mysql-1
      namespace: default
      resourceVersion: "727134"
      uid: d9b349a8-ceca-41f2-92fc-0df100dfbc21
    zone: shanghai
  ports:
  - name: default
    port: 3306
    protocol: TCP
  type: Headless
```

这时可以在本集群的任意pod通过域名`mysql.default.svc.global`访问mysql这个全局服务了,  如果需要访问某个集群的myql的特定pod，比如chaoyang集群的mysql-0, 可以用`mysql-0.chaoyang.mysql.default.svc.global`这个域名去访问。
