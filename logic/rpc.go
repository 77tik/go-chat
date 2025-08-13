package logic

import (
	"fmt"
	"github.com/rcrowley/go-metrics"
	"github.com/rpcxio/rpcx-etcd/serverplugin"
	"github.com/sirupsen/logrus"
	"github.com/smallnest/rpcx/server"
	"gochat/config"
	"gochat/internal/tools"
	"strings"
	"time"
)

// 真正的RPC逻辑，需要被注册的
type RpcLogic struct {
}

// logic 层作为Server去处理API层的rpc call
func (logic *Logic) InitRpcServer() (err error) {
	var network, addr string
	// a host multi port case
	// 这是服务发现吗？
	rpcAddressList := strings.Split(config.Conf.Logic.LogicBase.RpcAddress, ",")
	for _, bind := range rpcAddressList {
		if network, addr, err = tools.ParseNetwork(bind); err != nil {
			logrus.Panicf("InitLogicRpc ParseNetwork error : %s", err.Error())
		}
		logrus.Infof("logic start run at-->%s:%s", network, addr)
		go logic.createRpcServer(network, addr)
	}
	return
}

func (logic *Logic) createRpcServer(network string, addr string) {
	s := server.NewServer()

	// 添加etcd？
	logic.addRegistryPlugin(s, network, addr)
	// serverId must be unique
	//err := s.RegisterName(config.Conf.Common.CommonEtcd.ServerPathLogic, new(RpcLogic), fmt.Sprintf("%s", config.Conf.Logic.LogicBase.ServerId))
	err := s.RegisterName(config.Conf.Common.CommonEtcd.ServerPathLogic, new(RpcLogic), fmt.Sprintf("%s", logic.ServerId))
	if err != nil {
		logrus.Errorf("register error:%s", err.Error())
	}

	// 服务注销流程
	s.RegisterOnShutdown(func(s *server.Server) {
		s.UnregisterAll()
	})

	s.Serve(network, addr)
}

func (logic *Logic) addRegistryPlugin(s *server.Server, network string, addr string) {
	r := &serverplugin.EtcdV3RegisterPlugin{
		ServiceAddress: network + "@" + addr,                         // 地址格式 tcp@192.168.1.100:6000
		EtcdServers:    []string{config.Conf.Common.CommonEtcd.Host}, // etcd集群地址
		BasePath:       config.Conf.Common.CommonEtcd.BasePath,       // 服务注册根路径
		Metrics:        metrics.NewRegistry(),                        // 监控指标
		UpdateInterval: time.Minute,                                  //心跳间隔
	}
	err := r.Start()
	if err != nil {
		logrus.Fatal(err)
	}
	s.Plugins.Add(r)
}
