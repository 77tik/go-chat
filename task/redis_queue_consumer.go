/**
 * Created by lock
 * Date: 2019-08-13
 * Time: 10:13
 */
package task

import (
	"github.com/go-redis/redis"
	"github.com/sirupsen/logrus"
	"gochat/config"
	"gochat/internal/tools"
	"time"
)

// 使用redis制作的消息队列，也许可以用kafka替代一波

// 我有个疑问，我在connect层似乎也有一个redis客户端，那么同时用的时候会冲突吗，我觉得应该不会？
var RedisClient *redis.Client

// 开启消费者协程
func (task *Task) InitQueueRedisClient() (err error) {
	redisOpt := tools.RedisOption{
		Address:  config.Conf.Common.CommonRedis.RedisAddress,
		Password: config.Conf.Common.CommonRedis.RedisPassword,
		Db:       config.Conf.Common.CommonRedis.Db,
	}
	RedisClient = tools.GetRedisInstance(redisOpt)
	if pong, err := RedisClient.Ping().Result(); err != nil {
		logrus.Infof("RedisClient Ping Result pong: %s,  err: %s", pong, err)
	}
	go func() {
		// 无线循环，持续消费消息
		for {
			var result []string
			//10s timeout
			result, err = RedisClient.BRPop(time.Second*10, config.QueueName).Result()
			if err != nil {
				logrus.Infof("task queue block timeout,no msg err:%s", err.Error())
			}

			// 为什么结果要 >= 2 这个消费的消息是有什么形式吗
			// BRPOP key [key ...] timeout 的返回是一个两元素数组：
			//
			//第一个是被弹出的列表名（哪个 key 被命中）；
			//
			//第二个是被弹出的值。
			//
			//即使你只传了一个队列名，它也会返回 []string{queueName, value}。
			//所以你原代码里 len(result) >= 2 就是为了确保能安全地取到 result[1] 这个消息体。
			if len(result) >= 2 {
				task.Push(result[1])
			}
		}
	}()
	return
}
