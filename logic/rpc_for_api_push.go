package logic

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gochat/config"
	proto2 "gochat/internal/proto"
	"gochat/internal/tools"
	"strconv"
	"strings"
)

/*
*
single send msg 发消息队列中
*/
func (rpc *RpcLogic) Push(ctx context.Context, args *proto2.Send, reply *proto2.SuccessReply) (err error) {
	reply.Code = config.FailReplyCode
	sendData := args
	var bodyBytes []byte
	bodyBytes, err = json.Marshal(sendData)
	if err != nil {
		logrus.Errorf("logic,push msg fail,err:%s", err.Error())
		return
	}

	// 获取接收者所在的Connection服务器层，这个存在redis中
	logic := new(Logic)
	userSidKey := logic.getUserKey(fmt.Sprintf("%d", sendData.ToUserId))
	serverIdStr := RedisSessClient.Get(userSidKey).Val()
	//var serverIdInt int
	//serverIdInt, err = strconv.Atoi(serverId)
	if err != nil {
		logrus.Errorf("logic,push parse int fail:%s", err.Error())
		return
	}

	// 推送到对应的队列中
	// err = logic.RedisPublishChannel(serverIdStr, sendData.ToUserId, bodyBytes)
	err = logic.KafkaPublishChannel(serverIdStr, sendData.ToUserId, bodyBytes)
	if err != nil {
		logrus.Errorf("logic,redis publish err: %s", err.Error())
		return
	}
	reply.Code = config.SuccessReplyCode
	return
}

/*
*
push msg to room 群聊消息推送 到队列中
*/
func (rpc *RpcLogic) PushRoom(ctx context.Context, args *proto2.Send, reply *proto2.SuccessReply) (err error) {
	reply.Code = config.FailReplyCode

	// --- 新增：识别 /ai /summarize /translate ---
	msg := strings.TrimSpace(args.Msg)
	if strings.HasPrefix(msg, "/ai ") || strings.HasPrefix(msg, "/summarize") || strings.HasPrefix(msg, "/translate ") {
		job := &AIJob{
			RoomID:     args.RoomId,
			FromUserID: args.FromUserId,
			FromName:   args.FromUserName,
		}
		switch {
		case strings.HasPrefix(msg, "/ai "):
			job.Op = "ask"
			job.Prompt = strings.TrimSpace(strings.TrimPrefix(msg, "/ai "))

		case strings.HasPrefix(msg, "/summarize"):
			job.Op = "summarize"
			// 如果你允许带参数（如 /summarize 50），可在此解析并放进 job.Prompt

		case strings.HasPrefix(msg, "/translate "):
			job.Op = "translate"
			tail := strings.TrimSpace(strings.TrimPrefix(msg, "/translate "))
			parts := strings.SplitN(tail, " ", 2) // 期望：/translate en 这句中文
			if len(parts) >= 2 {
				job.Lang = parts[0]
				job.Prompt = parts[1]
			} else {
				// 不合法的用法，直接返回提示或走默认翻译
				job.Lang = "en"
				job.Prompt = tail
			}
		}
		// 写入 AI 任务队列
		l := new(Logic)
		if err := l.KafkaPublishAIJob(job); err != nil {
			logrus.Errorf("PushRoom publish AI job err: %v", err)
			return err
		}
		reply.Code = config.SuccessReplyCode
		// 可选：reply.Msg = "ok"（你的 proto.SuccessReply 里是否带 Msg 字段）
		return nil
	}

	sendData := args
	roomId := sendData.RoomId
	logic := new(Logic)
	roomUserInfo := make(map[string]string)
	roomUserKey := logic.getRoomUserKey(strconv.Itoa(roomId))
	roomUserInfo, err = RedisClient.HGetAll(roomUserKey).Result()
	if err != nil {
		logrus.Errorf("logic,PushRoom redis hGetAll err:%s", err.Error())
		return
	}
	//if len(roomUserInfo) == 0 {
	//	return errors.New("no this user")
	//}
	var bodyBytes []byte
	sendData.RoomId = roomId
	sendData.Msg = args.Msg
	sendData.FromUserId = args.FromUserId
	sendData.FromUserName = args.FromUserName
	sendData.Op = config.OpRoomSend
	sendData.CreateTime = tools.GetNowDateTime()

	sendData.ClientMsgId = tools.GetSnowflakeIdForInt64()
	bodyBytes, err = json.Marshal(sendData)
	if err != nil {
		logrus.Errorf("logic,PushRoom Marshal err:%s", err.Error())
		return
	}

	// 推队列
	// err = logic.RedisPublishRoomInfo(roomId, len(roomUserInfo), roomUserInfo, bodyBytes)
	err = logic.KafkaPublishRoomInfo(roomId, len(roomUserInfo), roomUserInfo, bodyBytes)
	if err != nil {
		logrus.Errorf("logic,PushRoom err:%s", err.Error())
		return
	}
	reply.Code = config.SuccessReplyCode
	return
}

/*
*
get room online person count 获取房间在线人数信息
*/
func (rpc *RpcLogic) Count(ctx context.Context, args *proto2.Send, reply *proto2.SuccessReply) (err error) {
	reply.Code = config.FailReplyCode
	roomId := args.RoomId
	logic := new(Logic)
	var count int
	count, err = RedisSessClient.Get(logic.getRoomOnlineCountKey(fmt.Sprintf("%d", roomId))).Int()

	// 推队列
	// err = logic.RedisPushRoomCount(roomId, count)
	err = logic.KafkaPushRoomCount(roomId, count)
	if err != nil {
		logrus.Errorf("logic,Count err:%s", err.Error())
		return
	}
	reply.Code = config.SuccessReplyCode
	return
}

/*
*
get room info
*/
func (rpc *RpcLogic) GetRoomInfo(ctx context.Context, args *proto2.Send, reply *proto2.SuccessReply) (err error) {
	reply.Code = config.FailReplyCode
	logic := new(Logic)
	roomId := args.RoomId
	roomUserInfo := make(map[string]string)
	roomUserKey := logic.getRoomUserKey(strconv.Itoa(roomId))
	roomUserInfo, err = RedisClient.HGetAll(roomUserKey).Result()
	if len(roomUserInfo) == 0 {
		return errors.New("getRoomInfo no this user")
	}

	// 推队列
	// err = logic.RedisPushRoomInfo(roomId, len(roomUserInfo), roomUserInfo)
	err = logic.KafkaPushRoomInfo(roomId, len(roomUserInfo), roomUserInfo)
	if err != nil {
		logrus.Errorf("logic,GetRoomInfo err:%s", err.Error())
		return
	}
	reply.Code = config.SuccessReplyCode
	return
}
