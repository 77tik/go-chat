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


## Raft 算法
+ raft算法可以让一个集群组成一组复制状态机
  + 复制状态机：
    + 每个服务器是一个状态机，服务器的运行只能通过一行行的命令改变，这些命令被存储在日志中，日志的命令会被按顺序严格执行；
    + 所以如果每个状态机都按照相同的日志执行命令，那么他们最终都会一致，所以只要保证日志的一致性就可以做到整体的一致
  + raft一致性算法：
    + leader的作用：接收命令并将其作为日志条目赋值给其他服务器
      + 问题1：当已有leader故障的时候要选一个新的leader出来
      + 问题2：leader接收命令以后记录为日志，并强制其他节点的日志与leader一致
      + 问题3：怎么保证安全呢？
    + 基本概念：
      + 三个角色：leader，candidate，follower
        + follower只会响应leader和candidate的请求。客户端的请求即使来了也会被重定向到leader
        + candidate使候选者，只会出现在选举时，选举成功以后会成为新的leader
      + time out 信号：
        + 在集群开始的时候所有的节点都是follower，在time out信号的驱使下，所有的follower都会变成 candidate
        + candidate会去拉取选票，得票最多者变为leader，其余退回follower
        + 再另一个timeout发出时还没有选举好，就重新再来选举
      + term 时间片
        + 每一个term都从新的选举开始，一旦有candidate获胜，那么剩余的term时间内它就会保持leader状态，但也可以直至term结束都没有leader，下一个term来的时候重新发起选举
        + 逻辑时钟：每一个server都存储了当前term编号，在server之间交流的时候就会带着这个编号，如果一个server发现编号小于了另一个，那么它就会更新自己的编号为其
        + 如果leader或者candidate发现自己的编号不是最新的了，就会把自己转化为follower
        + 如果接收到的请求的 term编号小于自己的当前term，将会拒绝执行
        + term 是按递增编号排列为时间顺序：term1 term2 term3 ……
      + RPC：
        + server之间的交流都是RPC，需要建立最基本的两个RPC 就能构建一个Raft集群
        + RequestVote RPC：由选举过程中的candidate发起，用来拉选票
        + AppendEntries RPC：由leader发起，用于复制日志或者发送心跳信号
    + 选举：
      + follower
        + 选举是由follower发起的，如果收到了来自leader或candidate得到RPC，f就保持f状态，避免抢选票变为candidate
        + leader会发送空的 AppendEntries RPC作为心跳信号来确立自己的地位，如果f一段时间（timeout）没有收到心跳，那么他就认为leader似了，发起新的一轮选举
        + 如果follower发起了选举，它就会增加自己的term编号，并转变为candidate，会给自己投一票，然后向其他节点并行发起RequestVote RPC
      + candidate：
        + 如果它在一个term内收到了大多数的选票，将会在接下的剩余term时间内称为leader，然后就可以通过发送心跳确立自己的地位。(每一个server在一个term内只能投一张选票，并且按照先到先得的原则投出)
        + 其他server成为leader：在等待投票时，可能会收到其他server发出AppendEntries RPC心跳信号，说明其他leader已经产生了。这时通过比较自己的term编号和RPC过来的term编号，如果比对方大，说明leader的term过期了，就会拒绝该RPC,并继续保持候选人身份; 如果对方编号不比自己小,则承认对方的地位,转为follower.
        + 选票被瓜分,选举失败: 如果没有candidate获取大多数选票, 则没有leader产生, candidate们等待超时后发起另一轮选举. *为了防止下一次选票还被瓜分*,必须采取一些额外的措施, raft采用随机election timeout的机制防止选票被持续瓜分。通过将timeout随机设为一段区间上的某个值, 因此很大概率会有某个candidate率先超时然后赢得大部分选票.
    + 日志复制：
      + leader选举成功以后就要对客户端提供服务，客户端的每一条请求都会被leader按顺序记录到日志中，包含term编号和顺序索引
      + 然后leader就并行向所有节点发送AppendEntries RPC用以复制命令，会包含leader刚刚处理的一条命令，接收节点如果发现上一条命令不匹配就拒绝执行
      + 当大多数节点复制成功 以后，leader会提交该命令，即执行命令并将结果返回给客户端
    + 日志一致性：
      + leader强制follower复制自己的日志，leader会找到最新的 大家都一致的条目，然后让所有follower把该条目之后的全删了，
      + leader再一股脑把自己在那之后的日志条目一股脑全部推送给所有follower
      + 寻找该条目可以通过AppendEntries RPC，该RPC中包含着下一次要执行的命令索引，如果能和follower的当前索引对上，那就执行，否则拒绝，然后leader将会逐次递减索引，直到找到相同的那条日志。
    + 日志一致性衍生问题：如果让一个菜b成为了leader，那么其不会让大家都强制变得一样菜
      + Raft通过为选举过程添加一个限制条件，解决了上面提出的问题，该限制确保leader包含之前term已经提交过的所有命令。Raft通过投票过程确保只有拥有全部已提交日志的candidate能成为leader。
      + 由于candidate为了拉选票需要通过RequestVote RPC联系其他节点，而之前提交的命令至少会存在于其中某一个节点上,因此只要candidate的日志至少和其他大部分节点的一样新就可以了, follower如果收到了不如自己新的candidate的RPC,就会将其丢弃.
    + 日志一致性衍生问题的衍生问题：如果leader提交的时候好巧不巧就崩了，但是此时命令已经被复制到大部分节点上了
      + 新的leader必须完成这未竟的事业，把之前term没有完成的提交了，Raft让leader查看当前term内没有完成的提交是否已经被复制半数以上，是就提交了
    + 日志压缩：
      + Snapshotting是最简单的压缩方法，系统的全部状态会写入一个snapshot保存起来，然后丢弃截止到snapshot时间点之前的所有日志
      + 虽然每一个server都保存有自己的snapshot，但是当follower严重落后于leader时，leader需要把自己的snapshot发送给follower加快同步，此时用到了一个新的RPC：InstallSnapshot RPC。
      + follower收到snapshot时，需要决定如何处理自己的日志，如果收到的snapshot包含有更新的信息，它将丢弃自己已有的日志，按snapshot更新自己的状态，如果snapshot包含的信息更少，那么它会丢弃snapshot中的内容，但是自己之后的内容会保存下来。



## Redis

+ 本项目的redis作用为两个：
  + 存储会话，用户信息
  + 作为消息队列
+ 存储会话与用户信息：