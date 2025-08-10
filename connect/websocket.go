/**
 * Created by lock
 * Date: 2019-08-09
 * Time: 15:19
 */
package connect

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	"gochat/config"
	"gochat/proto"
	"net/http"
	"time"
)

func (c *Connect) InitWebsocket() error {
	// 注册ws路由
	// 这个默认Server是connect.go 里面的，初始化的时候把他初始化了，connect是个铁废物，只知道依靠别人
	// 咦，我注册了这个路由，那不就解决了多个客户端的连接问题了吗，只要有一个客户端连接，我就进行一次serveWs
	// 然后每个客户端都是独立的，独立的ch，独立的读写通道
	// 然后通过读通道把连接放入Server的筒子里面，如果传来的roomId不存在，就创建一个Room把连接放进去
	// Server不是谁管理的问题，而是程序开始的时候它就已经在了，随着程序共存亡，Connect就是个寄生虫，计生在这个Server上
	// 有Server就有Connect
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		c.serveWs(DefaultServer, w, r)
	})
	// 在配置地址上启动http服务
	err := http.ListenAndServe(config.Conf.Connect.ConnectWebsocket.Bind, nil)
	return err
}

func (c *Connect) serveWs(server *Server, w http.ResponseWriter, r *http.Request) {

	// 创建连接升级
	var upGrader = websocket.Upgrader{
		ReadBufferSize:  server.Options.ReadBufferSize,  // 读缓冲
		WriteBufferSize: server.Options.WriteBufferSize, // 写缓冲
	}
	//cross origin domain support
	// 允许跨域
	upGrader.CheckOrigin = func(r *http.Request) bool { return true }

	// 升级HTTP连接到WebSocket
	conn, err := upGrader.Upgrade(w, r, nil)

	if err != nil {
		logrus.Errorf("serverWs err:%s", err.Error())
		return
	}
	var ch *Channel
	//default broadcast size eq 512
	// 这不能就一个连接吧？？？
	// 当然，初始化的时候我容许你只有一个
	ch = NewChannel(server.Options.BroadcastSize)
	ch.conn = conn
	// 读写通道同时开启
	// 读通道是读取客户端传来的信息，客户端必须必须必须必须必须传connectRequest，少一个给你杀了
	// 而且我也没看到消息，因为消息是api接口放到广播通道里面的，别tm客户端传消息过来了行吗，不会叼你的
	// 写通道就是读取广播通道中的消息然后发走，不会反序列化的老弟，广播通道里面又不是byte数组，你这么多心干嘛
	// 直接拿到msg结构体，然后把结构体的msg字段给你发走了，还管什么序列化反序列化的？
	//send data to websocket conn
	go server.writePump(ch, c)
	//get data from websocket conn
	go server.readPump(ch, c)
}

// tcp同款写通道，不过似乎有一些变化
// 那么是谁忘写通道里面写东西了呢，是接口吗？还是客户端自己写的
// 是读取广播通道的消息，那么是广播通道往写通道中写东西，那么是谁往广播通道中写东西呢
func (s *Server) writePump(ch *Channel, c *Connect) {
	//PingPeriod default eq 54s
	ticker := time.NewTicker(s.Options.PingPeriod)
	defer func() {
		ticker.Stop()
		ch.conn.Close()
	}()
	// 1.变化：没有打包发送？
	for {
		select {
		case message, ok := <-ch.broadcast:
			//write data dead time , like http timeout , default 10s
			ch.conn.SetWriteDeadline(time.Now().Add(s.Options.WriteWait))
			if !ok {
				logrus.Warn("SetWriteDeadline not ok")
				ch.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// 消息分帧？
			w, err := ch.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				logrus.Warn(" ch.conn.NextWriter err :%s  ", err.Error())
				return
			}
			logrus.Infof("message write body:%s", message.Body)
			w.Write(message.Body)
			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			//heartbeat，if ping error will exit and close current websocket conn
			ch.conn.SetWriteDeadline(time.Now().Add(s.Options.WriteWait))
			logrus.Infof("websocket.PingMessage :%v", websocket.PingMessage)
			if err := ch.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// 读通道是读取的谁？是客户端发送来的消息吗？
// 还真是，还真是原样读取客户端发来的信息的，只不过客户端发来的是connect token
// 然后顺手给token 加redis中了（这里当然是rpc调用logic的方法）
// 居然从外部传进来connect 吗？然后拿到请求的 roomId和ServerId，put进筒子吗
// 那么这到底是谁传来的connect？
// 真服了，原来是connect层自己传进来的，那它权利倒挺大了，他自己就是connect，有serverId
// 结果还不满足，还要把自己传进去
// 我们启动connect层实例的时候，就已经初始化了一个connect对象，然后这个connect为每个连接建立入桶
// 筒子全都放在Server里面，那么Server在哪？
func (s *Server) readPump(ch *Channel, c *Connect) {
	defer func() {
		logrus.Infof("start exec disConnect ...")
		if ch.Room == nil || ch.userId == 0 {
			logrus.Infof("roomId and userId eq 0")
			ch.conn.Close()
			return
		}
		logrus.Infof("exec disConnect ...")
		disConnectRequest := new(proto.DisConnectRequest)
		disConnectRequest.RoomId = ch.Room.Id
		disConnectRequest.UserId = ch.userId
		s.Bucket(ch.userId).DeleteChannel(ch)
		if err := s.operator.DisConnect(disConnectRequest); err != nil {
			logrus.Warnf("DisConnect err :%s", err.Error())
		}
		ch.conn.Close()
	}()

	ch.conn.SetReadLimit(s.Options.MaxMessageSize)
	// 设置读超时
	ch.conn.SetReadDeadline(time.Now().Add(s.Options.PongWait))

	// 这是在干嘛？
	ch.conn.SetPongHandler(func(string) error {
		ch.conn.SetReadDeadline(time.Now().Add(s.Options.PongWait))
		return nil
	})

	for {
		_, message, err := ch.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logrus.Errorf("readPump ReadMessage err:%s", err.Error())
				return
			}
		}
		if message == nil {
			return
		}
		var connReq *proto.ConnectRequest
		logrus.Infof("get a message :%s", message)
		if err := json.Unmarshal([]byte(message), &connReq); err != nil {
			logrus.Errorf("message struct %+v", connReq)
		}
		if connReq == nil || connReq.AuthToken == "" {
			logrus.Errorf("s.operator.Connect no authToken")
			return
		}
		connReq.ServerId = c.ServerId //config.Conf.Connect.ConnectWebsocket.ServerId
		userId, err := s.operator.Connect(connReq)
		if err != nil {
			logrus.Errorf("s.operator.Connect error %s", err.Error())
			return
		}
		if userId == 0 {
			logrus.Error("Invalid AuthToken ,userId empty")
			return
		}
		logrus.Infof("websocket rpc call return userId:%d,RoomId:%d", userId, connReq.RoomId)
		// 我们取一个Server管理的筒子，然后把连接放进去
		b := s.Bucket(userId)
		//insert into a bucket
		err = b.Put(userId, connReq.RoomId, ch)
		if err != nil {
			logrus.Errorf("conn close err: %s", err.Error())
			ch.conn.Close()
		}
	}
}
