/**
 * Created by lock
 * Date: 2019-08-09
 * Time: 15:18
 */
package connect

import (
	"gochat/proto"
	"sync"
	"sync/atomic"
)

type Bucket struct {
	cLock           sync.RWMutex     // protect the channels for chs
	chs             map[int]*Channel //  用户ID => 连接映射,这个连接不是Connect这个废物，记住Connect这个废物是一点用都没有的
	bucketOptions   BucketOptions
	rooms           map[int]*Room                    //  房间ID => 房间对象映射
	ChToPushRoomMsg []chan *proto.PushRoomMsgRequest // 广播携程用到的通道,每个通道都能独立做到，为了不让一个通道负担过重，所以这里用了多个，然后轮训即可
	routinesNum     uint64                           // 轮询计数器
}

type BucketOptions struct {
	ChannelSize          int    // 初始链接容量
	RoomSize             int    // 初始房间容量
	ChToPushRoomMsgCount uint64 // 广播协程数量？ 这真是广播协程吗？ 是的
	RoutineSize          int    // 广播队列大小 ？
}

func NewBucket(bucketOptions BucketOptions) (b *Bucket) {
	b = new(Bucket)
	// 根据设置的链接容量初始化链接数
	b.chs = make(map[int]*Channel, bucketOptions.ChannelSize)
	b.bucketOptions = bucketOptions

	// 根据广播携程数初始化管道数量
	b.ChToPushRoomMsg = make([]chan *proto.PushRoomMsgRequest, bucketOptions.ChToPushRoomMsgCount)
	b.rooms = make(map[int]*Room, bucketOptions.RoomSize)

	// 启动广播协程
	for i := uint64(0); i < b.bucketOptions.ChToPushRoomMsgCount; i++ {

		// 根据通道的容量设置初始每个通道大小
		c := make(chan *proto.PushRoomMsgRequest, bucketOptions.RoutineSize)
		// 挂载上去
		b.ChToPushRoomMsg[i] = c

		// 启动广播协程，for循环监控管道是否有消息，有消息就广播推送（和zinx的实现一样）
		// 这里调用PushRoom你以为是调用Room对象的Push方法吗，Room说到底就是个管理链表的头节点而已
		// 它何德何能能发消息？发消息最自然是我们的连接发的，不是Connect那个废物！
		// 连接吧消息放到连接自己的broadcast内，然后写通道到直接读取broadcast就可以了
		go b.PushRoom(c)
	}
	return
}

func (b *Bucket) PushRoom(ch chan *proto.PushRoomMsgRequest) {
	for {
		var (
			arg  *proto.PushRoomMsgRequest
			room *Room
		)
		arg = <-ch
		if room = b.Room(arg.RoomId); room != nil {
			room.Push(&arg.Msg)
		}
	}
}

// 根据roomid 获取关联的room对象
func (b *Bucket) Room(rid int) (room *Room) {
	b.cLock.RLock()
	room, _ = b.rooms[rid]
	b.cLock.RUnlock()
	return
}

// 添加链接？将用户/链接/房间 入桶管理
func (b *Bucket) Put(userId int, roomId int, ch *Channel) (err error) {
	var (
		room *Room
		ok   bool
	)
	b.cLock.Lock()

	// 房间管理，添加新房间
	if roomId != NoRoom {
		if room, ok = b.rooms[roomId]; !ok {
			room = NewRoom(roomId)
			b.rooms[roomId] = room
		}
		ch.Room = room
	}
	// 关联链接，有个疑问，链接已经被存储到room中了，干嘛还要单独存一个链接关联呢，可能是找的方便吧，待解释
	ch.userId = userId
	b.chs[userId] = ch
	b.cLock.Unlock()

	// 将链接添加到房间中
	if room != nil {
		err = room.Put(ch)
	}
	return
}

// 原来是为了删除方便
func (b *Bucket) DeleteChannel(ch *Channel) {
	var (
		ok   bool
		room *Room
	)
	b.cLock.RLock()
	if ch, ok = b.chs[ch.userId]; ok {
		room = b.chs[ch.userId].Room
		//delete from bucket
		delete(b.chs, ch.userId)
	}
	if room != nil && room.DeleteChannel(ch) {
		// if room empty delete,will mark room.drop is true
		// 如果房间为空，那就删掉这个房间
		if room.drop == true {
			delete(b.rooms, room.Id)
		}
	}
	b.cLock.RUnlock()
}

// 返回userid 对应的链接
func (b *Bucket) Channel(userId int) (ch *Channel) {
	b.cLock.RLock()
	ch = b.chs[userId]
	b.cLock.RUnlock()
	return
}

// 广播消息，轮询到处理广播的协程中，往channel中砸msg，仅仅只是砸，应该还是要让协程去处理的
func (b *Bucket) BroadcastRoom(pushRoomMsgReq *proto.PushRoomMsgRequest) {
	num := atomic.AddUint64(&b.routinesNum, 1) % b.bucketOptions.ChToPushRoomMsgCount
	b.ChToPushRoomMsg[num] <- pushRoomMsgReq
}
