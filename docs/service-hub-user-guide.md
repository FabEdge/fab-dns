# service-hub使用指南

service-hub负责在多个集群中交换GlobalService, 确保每个集群中的GlobalService数据一致。

service-hub有两种模式：server/client. 作为server的service-hub只能有一个，其他service-hub都必须以client模式跟它交互。不管是以哪种模式运行，都需要TLS证书和私钥，这些证书必须由同一个CA证书签发。

## 参数说明

* mode: service-hub的启动模式，只有两个可选项: server/client, 默认值server。
* cluster: service-hub所在集群的名称，必须配置。cluster, zone, region三者都是集群的拓扑信息，每个GlobalService的端点都会包含这些信息，这些信息必须与fabdns组件的配置相同。
* zone: service-hub所在集群的所在zone， 必须配置。
* region: service-hub所在集群的region.，必须配置。
* health-probe-listen-address: 健康检测探针地址，默认值: 0.0.0.0:3001. 
* api-server-listen-address: API Server监听地址, 仅在server模式下起作用， 默认值: 0.0.0.0:3000
* api-server-address: API Server地址，仅在client模式下起作用，必须配置. 例子: https://10.40.20.181:3000/
* tls-key-file: TLS私钥文件路径，文件必须是PEM格式, 必须配置
* tls-cert-file: TLS证书文件路径，文件必须是PEM格式，必须配置
* tls-ca-cert-file: 签发证书的CA证书文件，文件必须是PEM格式，必须配置
* cluster-expire-duration: 集群过期时间, 仅在server模式下起作用, 默认值5分钟。当一个client service-hub停止向 server service-hub发送心跳后，当达到cluster-expire-duration后，server service-hub会将相关的集群提供的全局服务删除。
* service-import-interval: 全局服务导入间隔，仅在client模式下起作用, 默认值一分钟。
* allow-create-namespace: 是否允许创建namespace, 默认值true. 当值为false且缺失相关namespace时，会导致有些有些服务导入失败。
