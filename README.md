## RpcX + Etcd
+ rpcx的serverPlugin已经集成好了etcd的注册和发现流程：
``` go
plugin := &serverplugin.EtcdRegisterPlugin{
    ServiceAddress: "tcp@" + addr, // 服务监听地址
    EtcdServers:    []string{"127.0.0.1:2379"},
    BasePath:       "/rpcx",
    UpdateInterval: time.Minute,  // 续租间隔
}
plugin.Start()
s.Plugins.Add(plugin)

```
+ 服务端启动时注册到 etcd

  + 写入 /rpcx/<服务名>/<节点ID> 这样的 key。
  + 附带 IP、端口、元数据（版本、标签等）。

  + 定期续租，服务掉线会自动从 etcd 删除。

+ 客户端自动订阅服务

  + 如果你用 client.NewEtcdDiscovery 或 client.NewEtcdV3Discovery，它会去 etcd 订阅某个 BasePath 下的 key。

  + 有变动会实时更新本地服务节点列表。

+ 客户端负载均衡 & 连接管理

  + 发现多个服务节点后，客户端会用 round-robin、随机、权重等策略选一个。

  + 连接是长连接（默认 TCP + 自定义协议），不会每次调用都去 etcd。

+ 方法调用

  + 服务端通过 s.RegisterName 把方法暴露出来。

  + 客户端直接 xclient.Call(ctx, "方法名", args, reply) 调用，rpcx 内部会序列化/发送/反序列化。

+ 全程不需要你手动查 etcd 或维护连接。
+ 服务端和客户端必须都同时使用rpcx框架才可以
+ Rpcx支持多种序列化反序列化类型，本项目使用的是默认的序列化类型，如果客户端没有设置的话，应该是msgPack
  + 但其实是可以指定的，如果你用的是protobuf生成代码，那么就需要制定序列化类型为Protobuf
  + 序列化类型通常由客户端在 client options 里设置（opt.SerializeType），客户端会把这个类型写到请求的协议头里。服务端在收到请求时根据消息头里的 SerializeType 来选用对应的 codec 去解码。
```
import (
    "github.com/smallnest/rpcx/client"
    "github.com/smallnest/rpcx/protocol"
)

// ...
opt := client.DefaultOption
opt.SerializeType = protocol.ProtoBuffer // 或 protocol.JSON / protocol.MsgPack / protocol.SerializeNone
xclient := client.NewXClient("YourService", client.Failtry, client.RoundRobin, discovery, opt)
```
+ 如果你要修改方法名的话，特别是rpc注册的方法要格外注意，你还得去调用该方法的地方把这个名字给改了

## Redis

+ 本项目的redis作用为两个：
  + 存储会话，用户信息
  + 作为消息队列
+ 存储会话与用户信息：
  + 