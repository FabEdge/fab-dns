# 基于域名解析的多集群服务发现

## 概述

[FabEdge](fabedge)使得多个边缘集群间的通信成为可能，一个集群的Pod可以访问另一个集群的Pod或服务，但这种访问必须使用IP地址，想通过域名访问则无能为力，因为Kubernetes自身没有提供跨集群的域名解析服务。Kubernetes社区已经开始着手解决这个问题，[KEP-1645: Multi-Cluster Services API](KEP-1645)和[mcs-api](mcs-api)便是社区的一个尝试，除此之外还有[Submariner](lighthouse)及[Cilium](cilium-service-discovery).

但Submariner和Cilium并不适合多边缘集群场景，Submariner依赖于其他的CNI，但这些CNI本身就无法解决边缘集群的通信问题，Cilium是全栈式解决方案，并且也无法实现边缘集群的内部通信。

FabDNS尝试在提供一个边缘场景下的多集群基于DNS的服务发现解决方案。



## 目标

* 允许一个集群访问其他集群提供的服务，服务类型仅限于ClusterIP, Headless两种。服务可以部署于一个集群内部，也可以分散在多个集群里。

* 提供一定的拓扑感知的DNS解析，访问者可以就近访问最近的服务节点。

  

## 术语

* 多集群 —— 一个通过FabEdge实现了相互通信的多个集群的集合，集群内部会有一个主集群(Host Cluster)和多个成员集群(Member Cluster)。 一个多集群内，可能会被社区划分为不同的几个通信社区，也可能彼此相通，但都要接受Host集群的管理。
* 集群名——一个集群的唯一标识
* 拓扑信息—— 一个集群的物理地址标识，有Zone和Region两种。
* 服务—— Kubernetes里的Service资源
* 端点—— 一个服务的后端，FabDNS从服务关联的EndpointSlice里获取这些信息

## 设计



### 导出服务

当一个集群向其他集群提供服务时，需要公开自己的服务。做法很简单：在服务的Annotations里添加一个标记:

`fabedge.io/global-service: "true"`， 例如：

```
apiVersion: v1
kind: Service
metadata:
  name: nginx
  annotations:
  	fabedge.io/global-service: "true"
spec:
  selector:
    app: nginx
  ports:
    - protocol: TCP
      port: 80
      targetPort: 80
```



有时多个集群可能会同时暴露同命名空间下的同名服务，这时我们认为这些服务组成了

一个全局服务。

当一个服务被它所在的集群公开后，这个服务还需要一些同步过程

* 向Host集群上传自己的服务信息，Host集群以GlobalService的形式保存这个服务。
* 其他集群从Host集群导入这些服务

这些同步活动由同步组件来完成。



### 全局服务——GlobalService

GlobalService是一个CRD，用来表示一个或多个被暴露出来的服务，它的数据结构如下：

```go
type GlobalService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec GlobalServiceSpec `json:"spec,omitempty"`
}

// GlobalServiceSpec describes global service and the information necessary to consume it.
type GlobalServiceSpec struct {
	// Must be ClusterIP or Headless
	Type ServiceType `json:"type,omitempty"`

	Ports []ServicePort `json:"ports,omitempty"`

	Endpoints []Endpoint `json:"endpoints,omitempty"`
}

// Endpoint represents a single logical "backend" implementing a service.
type Endpoint struct {
	Addresses []string `json:"addresses"`
    Hostname *string `json:"hostname,omitempty"`
	TargetRef *corev1.ObjectReference `json:"targetRef,omitempty"`
	
	Cluster string `json:"cluster,omitempty"`
	// Zone indicates the zone where the endpoint is located
	Zone string `json:"zone,omitempty"`
	// Region indicates the region where the endpoint is located
	Region string `json:"region,omitempty"`
}

// ServicePort represents the port on which the service is exposed
type ServicePort struct {
	Name string `json:"name,omitempty"`
	Protocol corev1.Protocol `json:"protocol,omitempty"`
	AppProtocol *string `json:"appProtocol,omitempty"`
	Port int32 `json:"port,omitempty"`
}
```

从上面的数据结构可以看出GlobalService是一个跟Service结构很相似的资源，但有个大的区别，它的端点(Endpoint)数据不是保存在EndpointSlice，而是直接写在Spec里。

GlobalService服务类型有两种:

* ClusterIP. 这意味着一个GlobalService背后的服务的类型也是ClusterIP, 它的端点信息也是这些服务。
* Headless. 这意味着一个GlobalService背后的服务是无头服务，因为一个无头服务本身没有ClusterIP, 那么全局服务的端点就是这些无头服务背后的Pod

因为一个GlobalService是由一个或多个服务组成的，所以这些组成的服务就必须要有同样的定义。如果一个集群被公开的服务，跟GlobalService的定义不同，那么该服务不会被视为一个端点。

GlobalService的端点除了包含有地址，主机名和资源类型这种信息外，还包含一些拓扑信息： Zone和Region。这些信息会在DNS解析时用到。



### DNS解析

当一个全局服务被创建后，就会获得一个可在多集群范围内可访问的域名，格式如下： `<service>.<ns>.svc.global`。

DNS解析由一个DNS组件负责，coredns会将后缀为global的解析请求转发给该组件。该组件每个集群都部署一个，部署时要配置cluster, zone, region等信息。

当DNS组件解析域名时，它会找到跟域名相关的全局服务，然后按如下流程找到合适的端点：

* 找到zone匹配的端点，生成响应，否则进入下一步;
* 找到region匹配的端点，生成响应，否则进入下一步;
* 将所有的端点地址都返回。



当一个全局服务是headless时，除了对服务的域名进行解析，还可以为每个端点进行域名解析，端点的域名格式为:

`<hostname>.<clustername>.<service>.<ns>.svc.global`。



### 心跳

服务同步组件除了导出导入全局服务信息外，还需要定时向Host集群发起心跳，这样Host的同步组件才会知道该集群的端点信息是有效的，否则当停止接受成员集群的心跳一段时间后，它会将该集群的信息从全局服务里清除。



[fabedge]: https://github.com/FabEdge/fabedge
[KEP-1645]: https://github.com/kubernetes/enhancements/tree/master/keps/sig-multicluster/1645-multi-cluster-services-api
[mcs-api]: https://github.com/kubernetes-sigs/mcs-api
[lighthouse]: https://submariner.io/getting-started/architecture/service-discovery/
[cilium-service-discovery]: https://submariner.io/getting-started/architecture/service-discovery/