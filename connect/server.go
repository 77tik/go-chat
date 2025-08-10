/**
 * Created by lock
 * Date: 2019-08-10
 * Time: 18:32
 */
package connect

import (
	"fmt"
	"gochat/tools"
	"time"
)

// TODO：可以用函数注入方式写的更装一些
type Server struct {
	Buckets []*Bucket // 使用多个筒子分散存储连接，减少锁竞争？具体用法我来看看怎么实现的
	// 但是目前怎么就一个筒子，他会扩容吗？
	// 我看懂了，每个Bucket会管理一些ROOM，ROOM则以链表形式管理Channel
	// 而且Buket也会直接记录userID => Channel的关联，方便删除，和找到其他Channel
	Options     ServerOptions // 服务器配置
	bucketCount uint32        // 筒子数量，待改名？
	operator    Operator      // RPC操作接口？ 具体是啥还得看一下
	// 我看完了，这是用来调用logic层在etcd注册的方法的，目前我们在Connection层只能调用加入房间和离开房间两个方法
	// 所以它是一个RPC操作符
}

type ServerOptions struct {
	WriteWait       time.Duration // 写超时
	PongWait        time.Duration // Pong响应超时？我记得Pong是在ping之后要的返回类型，那么这是否是用于心跳呢
	PingPeriod      time.Duration // 心跳间隔，这是留给tcp连接中服务器是不是会ping一下那一头
	MaxMessageSize  int64         // 最大消息大小
	ReadBufferSize  int           // 读缓冲
	WriteBufferSize int           // 写缓冲
	BroadcastSize   int           // 广播队列大小？？
}

// 用筒子数量 rpc操作符 服务器设置 来初始化服务器
func NewServer(b []*Bucket, o Operator, options ServerOptions) *Server {
	s := new(Server)
	s.Buckets = b
	s.Options = options
	s.bucketCount = uint32(len(b))
	s.operator = o
	return s
}

// reduce lock competition, use google city hash insert to different bucket
// 用奇怪的hash函数？算出hash值作为筒子索引，然后把这个筒子返回出去，似乎是要做一些操作
func (s *Server) Bucket(userId int) *Bucket {
	userIdStr := fmt.Sprintf("%d", userId)
	idx := tools.CityHash32([]byte(userIdStr), uint32(len(userIdStr))) % s.bucketCount
	return s.Buckets[idx]
}
