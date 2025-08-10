package connect

import (
	"context"
	"errors"
	"github.com/rpcxio/libkv/store"
	etcdV3 "github.com/rpcxio/rpcx-etcd/client"
	"github.com/sirupsen/logrus"
	"github.com/smallnest/rpcx/client"
	"gochat/config"
	"gochat/proto"
	"time"
)

// 沟通logic层的客户端，单例模式
var logicRpcClient client.XClient

// 初始化logic层客户端，为什么需要etcd呢？
// 因为要调用logic层注册在etcd中的服务
func (c *Connect) InitLogicRpcClient() (err error) {
	// 初始化etcd配置
	etcdConfigOption := &store.Config{
		ClientTLS:         nil,
		TLS:               nil,
		ConnectionTimeout: time.Duration(config.Conf.Common.CommonEtcd.ConnectionTimeout) * time.Second,
		Bucket:            "",
		PersistConnection: true,
		Username:          config.Conf.Common.CommonEtcd.UserName,
		Password:          config.Conf.Common.CommonEtcd.Password,
	}
	once.Do(func() {
		// 创建etcd服务发现
		d, e := etcdV3.NewEtcdV3Discovery(
			config.Conf.Common.CommonEtcd.BasePath,
			config.Conf.Common.CommonEtcd.ServerPathLogic,
			[]string{config.Conf.Common.CommonEtcd.Host},
			true,
			etcdConfigOption,
		)
		if e != nil {
			logrus.Fatalf("init connect rpc etcd discovery client fail:%s", e.Error())
		}
		// 创建RPC客户端
		logicRpcClient = client.NewXClient(
			config.Conf.Common.CommonEtcd.ServerPathLogic, // 服务发现路径
			client.Failtry,      // 失败重试策略
			client.RandomSelect, // 随机负载均衡
			d,
			client.DefaultOption,
		)
	})
	if logicRpcClient == nil {
		return errors.New("get rpc client nil")
	}
	return
}

// 面向对象写习惯了是吧
type RpcConnect struct {
}

// 加入房间（rpc调用logic层connect方法，logic初始化时已注册进etcd）
func (rpc *RpcConnect) Connect(connReq *proto.ConnectRequest) (uid int, err error) {
	reply := &proto.ConnectReply{}

	// 调用logic层的Connect方法，其实就是加入房间
	err = logicRpcClient.Call(context.Background(), "Connect", connReq, reply)
	if err != nil {
		logrus.Fatalf("failed to call: %v", err)
	}
	uid = reply.UserId
	logrus.Infof("connect logic userId :%d", reply.UserId)
	return
}

// 离开房间（rpc调用logic层disconnect方法，logic初始化时已注册进etcd）
func (rpc *RpcConnect) DisConnect(disConnReq *proto.DisConnectRequest) (err error) {
	reply := &proto.DisConnectReply{}
	if err = logicRpcClient.Call(context.Background(), "DisConnect", disConnReq, reply); err != nil {
		logrus.Fatalf("failed to call: %v", err)
	}
	return
}
