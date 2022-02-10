package lib

import (
	"errors"
	"fmt"
	"github.com/go-redis/redis"
	"strconv"
)

var redisClients map[int]*redis.Client

var pool *redis.Client

// type redisValue
type RedisValue struct {
	Key     string      `json:"key"`
	T       string      `json:"t"`
	Bit     bool        `json:"bit"`
	TTL     int         `json:"ttl"`
	Size    int         `json:"size"`
	Iterate int         `json:"iterate"`
	Val     interface{} `json:"val"`
}

func init() {
	redisClients = make(map[int]*redis.Client)
}

func printRedisPool(stats *redis.PoolStats) {
	fmt.Printf("Hits=%d Misses=%d Timeouts=%d TotalConns=%d IdleConns=%d StaleConns=%d\n",
		stats.Hits, stats.Misses, stats.Timeouts, stats.TotalConns, stats.IdleConns, stats.StaleConns)
}

func printRedisOption(opt *redis.Options) {
	fmt.Printf("Network=%v\n", opt.Network)
	fmt.Printf("Addr=%v\n", opt.Addr)
	fmt.Printf("Password=%v\n", opt.Password)
	fmt.Printf("DB=%v\n", opt.DB)
	fmt.Printf("MaxRetries=%v\n", opt.MaxRetries)
	fmt.Printf("MinRetryBackoff=%v\n", opt.MinRetryBackoff)
	fmt.Printf("MaxRetryBackoff=%v\n", opt.MaxRetryBackoff)
	fmt.Printf("DialTimeout=%v\n", opt.DialTimeout)
	fmt.Printf("ReadTimeout=%v\n", opt.ReadTimeout)
	fmt.Printf("WriteTimeout=%v\n", opt.WriteTimeout)
	fmt.Printf("PoolSize=%v\n", opt.PoolSize)
	fmt.Printf("MinIdleConns=%v\n", opt.MinIdleConns)
	fmt.Printf("MaxConnAge=%v\n", opt.MaxConnAge)
	fmt.Printf("PoolTimeout=%v\n", opt.PoolTimeout)
	fmt.Printf("IdleTimeout=%v\n", opt.IdleTimeout)
	fmt.Printf("IdleCheckFrequency=%v\n", opt.IdleCheckFrequency)
	fmt.Printf("TLSConfig=%v\n", opt.TLSConfig)

}

func getClient(serverID int, db int) (*redis.Client, error) {
	var ok bool
	pool, ok = redisClients[serverID]
	if ok == false {
		for i := 0; i < len(globalConfigs.Servers); i++ {
			if serverID == globalConfigs.Servers[i].ID {
				pool = redis.NewClient(&redis.Options{
					Addr:        globalConfigs.Servers[i].Host + ":" + strconv.Itoa(globalConfigs.Servers[i].Port), // Redis地址
					Password:    globalConfigs.Servers[i].Auth,                                                     // Redis账号
					DB:          db,                                                                                // Redis库
					IdleTimeout: -1,                                                                                //闲置超时，默认5分钟，-1表示取消闲置超时检查

					//连接池容量及闲置连接数量
					PoolSize:     10, // 连接池最大socket连接数，默认为4倍CPU数， 4 * runtime.NumCPU
					MinIdleConns: 5,  //在启动阶段创建指定数量的Idle连接，并长期维持idle状态的连接数不少于指定数量；。

					////超时
					//DialTimeout:  5 * time.Second, //连接建立超时时间，默认5秒。
					//ReadTimeout:  3 * time.Second, //读超时，默认3秒， -1表示取消读超时
					//WriteTimeout: 3 * time.Second, //写超时，默认等于读超时
					//PoolTimeout:  4 * time.Second, //当所有连接都处在繁忙状态时，客户端等待可用连接的最大等待时长，默认为读超时+1秒。
					//
					////闲置连接检查包括IdleTimeout，MaxConnAge
					//IdleCheckFrequency: 60 * time.Second,                                                    //闲置连接检查的周期，默认为1分钟，-1表示不做周期性检查，只在客户端获取连接时对闲置连接进行处理。
					//MaxConnAge:         0 * time.Second,                                                     //连接存活时长，从创建开始计时，超过指定时长则关闭连接，默认为0，即不关闭存活时长较长的连接
					//
					////命令执行失败时的重试策略
					//MaxRetries:      3,                      // 命令执行失败时，最多重试多少次，默认为0即不重试
					//MinRetryBackoff: 8 * time.Millisecond,   //每次计算重试间隔时间的下限，默认8毫秒，-1表示取消间隔
					//MaxRetryBackoff: 512 * time.Millisecond, //每次计算重试间隔时间的上限，默认512毫秒，-1表示取消间隔
					//
					////可自定义连接函数
					//Dialer: func() (net.Conn, error) {
					//	netDialer := &net.Dialer{
					//		Timeout:   5 * time.Second,
					//		KeepAlive: 5 * time.Minute,
					//	}
					//	return netDialer.Dial("tcp", "127.0.0.1:6379")
					//},
					//
					////钩子函数
					//OnConnect: func(conn *redis.Conn) error { //仅当客户端执行命令时需要从连接池获取连接时，如果连接池需要新建连接时则会调用此钩子函数
					//	fmt.Printf("conn=%v\n", conn)
					//	return nil
					//},
				})
			}
		}
	}

	_, err := pool.Ping().Result()
	if err == redis.Nil {
		return nil, errors.New("Redis异常")
	} else if err != nil {
		return nil, errors.New("失败:" + err.Error())
	} else {
		//fmt.Println(pong)
		//printRedisOption(pool.Options())
		//printRedisPool(pool.PoolStats())
	}

	redisClients[serverID] = pool
	c := pool
	if db >= 0 {
		_, err := c.Do("SELECT", db).Result()
		if err != nil {
			if c != nil {
				_ = c.Close()
			}
			return c, err
		}
	} else {
		_, err := c.Do("SELECT", 0).Result()
		if err != nil {
			if c != nil {
				_ = c.Close()
			}
			return nil, err
		}
	}

	return c, nil
}

func closeClient(serverID int) error {
	if client, ok := redisClients[serverID]; ok {
		return client.Close()
	}
	return nil
}

func testClient(name, address, password, port string) error {
	client := redis.NewClient(&redis.Options{
		Addr:     address + ":" + port,
		Password: password, // no password set
		DB:       0,        // use default DB
	})
	defer client.Close()

	pong, err := client.Ping().Result()
	if err == redis.Nil {
		return errors.New("Redis异常")
	} else if err != nil {
		return errors.New("失败:" + err.Error())
	} else {
		fmt.Println(pong)
	}
	return err
}
