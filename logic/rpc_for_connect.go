package logic

import (
	"context"
	"fmt"
	"github.com/sirupsen/logrus"
	"gochat/config"
	"gochat/internal/proto"
	"gochat/internal/tools"
	"strconv"
	"time"
)

// 加入房间
func (rpc *RpcLogic) Connect(ctx context.Context, args *proto.ConnectRequest, reply *proto.ConnectReply) (err error) {
	if args == nil {
		logrus.Errorf("logic,connect args empty")
		return
	}

	// 验证会话
	logic := new(Logic)
	//key := logic.getUserKey(args.AuthToken)
	logrus.Infof("logic,authToken is:%s", args.AuthToken)
	key := tools.GetSessionName(args.AuthToken)
	userInfo, err := RedisClient.HGetAll(key).Result()
	if err != nil {
		logrus.Infof("RedisCli HGetAll key :%s , err:%s", key, err.Error())
		return err
	}
	if len(userInfo) == 0 {
		reply.UserId = 0
		return
	}
	reply.UserId, _ = strconv.Atoi(userInfo["userId"])
	roomUserKey := logic.getRoomUserKey(strconv.Itoa(args.RoomId))
	if reply.UserId != 0 {
		userKey := logic.getUserKey(fmt.Sprintf("%d", reply.UserId))
		logrus.Infof("logic redis set userKey:%s, serverId : %s", userKey, args.ServerId)
		validTime := config.RedisBaseValidTime * time.Second

		// 记录用户 - 服务器 映射
		err = RedisClient.Set(userKey, args.ServerId, validTime).Err()
		if err != nil {
			logrus.Warnf("logic set err:%s", err)
		}

		// 加入房间，人数加1，房间记录新用户
		if RedisClient.HGet(roomUserKey, fmt.Sprintf("%d", reply.UserId)).Val() == "" {
			RedisClient.HSet(roomUserKey, fmt.Sprintf("%d", reply.UserId), userInfo["userName"])
			// add room user count ++
			RedisClient.Incr(logic.getRoomOnlineCountKey(fmt.Sprintf("%d", args.RoomId)))
		}
	}
	logrus.Infof("logic rpc userId:%d", reply.UserId)
	return
}

// 离开房间
func (rpc *RpcLogic) DisConnect(ctx context.Context, args *proto.DisConnectRequest, reply *proto.DisConnectReply) (err error) {
	logic := new(Logic)
	roomUserKey := logic.getRoomUserKey(strconv.Itoa(args.RoomId))
	// room user count -- 更新在线人数
	if args.RoomId > 0 {
		count, _ := RedisSessClient.Get(logic.getRoomOnlineCountKey(fmt.Sprintf("%d", args.RoomId))).Int()
		if count > 0 {
			RedisClient.Decr(logic.getRoomOnlineCountKey(fmt.Sprintf("%d", args.RoomId))).Result()
		}
	}
	// room login user-- 将用户从map中移除
	if args.UserId != 0 {
		err = RedisClient.HDel(roomUserKey, fmt.Sprintf("%d", args.UserId)).Err()
		if err != nil {
			logrus.Warnf("HDel getRoomUserKey err : %s", err)
		}
	}
	//below code can optimize send a signal to queue,another process get a signal from queue,then push event to websocket
	// 下方代码可优化为：发送信号到队列，再由另一个进程从队列获取信号并推送事件到WebSocket
	// 但是我看来这其实就已经推送到队列中，这个logic层的逻辑从来不自己处理逻辑，都是推送到队列中
	roomUserInfo, err := RedisClient.HGetAll(roomUserKey).Result()
	if err != nil {
		logrus.Warnf("RedisCli HGetAll roomUserInfo key:%s, err: %s", roomUserKey, err)
	}

	// 推队列
	//if err = logic.RedisPublishRoomInfo(args.RoomId, len(roomUserInfo), roomUserInfo, nil); err != nil {
	//	logrus.Warnf("publish RedisPublishRoomCount err: %s", err.Error())
	//	return
	//}
	if err = logic.KafkaPublishRoomInfo(args.RoomId, len(roomUserInfo), roomUserInfo, nil); err != nil {
		logrus.Warnf("publish RedisPublishRoomCount err: %s", err.Error())
		return
	}
	return
}
