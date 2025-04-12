package metaserver

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"sync"
	"time"
)

// 配置结构体（线程安全访问）
type Config struct {
	mu      *sync.RWMutex
	num     int
	version uint64
}

// RPC服务接口
type ConfigService struct {
	server      *MetaServer
	currentConf *Config // 原子存储*Config
}

// RPC请求参数
type ConfigRequest struct {
	ClientVersion uint64
}

// RPC响应结构
type ConfigResponse struct {
	Config   Config
	Version  uint64
	Modified bool
}

// 本地缓存结构
type LocalCache struct {
	mu       sync.RWMutex
	active   *Config
	fallback *Config
	strategy interface{} // 当前生效策略
}

// MetaServer主体
type MetaServer struct {
	rpcServer   *rpc.Server
	listener    net.Listener
	cache       LocalCache
	configChan  chan *Config
	stopChan    chan struct{}
	reloadCycle time.Duration
}

// 创建新配置服务
func NewMetaServer(port string, reloadInterval time.Duration) *MetaServer {
	ms := &MetaServer{
		rpcServer:   rpc.NewServer(),
		configChan:  make(chan *Config, 10),
		stopChan:    make(chan struct{}),
		reloadCycle: reloadInterval,
	}

	// 初始化RPC服务
	service := &ConfigService{server: ms}
	ms.rpcServer.RegisterName("MetaServer", service)

	// 初始化本地缓存
	initConfig := &Config{num: 100, version: 1}
	ms.cache.active = initConfig
	ms.cache.fallback = &Config{num: 50, version: 0}
	ms.cache.strategy = ms.generateStrategy(initConfig)

	// 启动网络监听
	go ms.startRPCServer(port)

	// 启动配置轮询
	go ms.configWatcher()

	return ms
}

// RPC服务方法
func (cs *ConfigService) GetConfig(req *ConfigRequest, resp *ConfigResponse) error {
	current := cs.server.cache.getActiveConfig()
	resp.Version = current.version

	if req.ClientVersion < current.version {
		resp.Config = *current
		resp.Modified = true
	} else {
		resp.Modified = false
	}
	return nil
}

// 获取当前活跃配置
func (lc *LocalCache) getActiveConfig() *Config {
	lc.mu.RLock()
	defer lc.mu.RUnlock()
	return lc.active
}

// 生成限流策略
func (ms *MetaServer) generateStrategy(c *Config) interface{} {
	// 实际业务逻辑示例：
	return struct {
		RateLimit int
		Threshold int
	}{
		RateLimit: c.num / 2,
		Threshold: c.num,
	}
}

// 启动RPC服务
func (ms *MetaServer) startRPCServer(port string) {
	var err error
	ms.listener, err = net.Listen("tcp", ":"+port)
	if err != nil {
		panic(fmt.Sprintf("RPC server start failed: %v", err))
	}

	for {
		conn, err := ms.listener.Accept()
		if err != nil {
			select {
			case <-ms.stopChan:
				return // 正常关闭
			default:
				fmt.Printf("Accept error: %v\n", err)
			}
			continue
		}
		go ms.rpcServer.ServeConn(conn)
	}
}

// 配置监控协程
func (ms *MetaServer) configWatcher() {
	ticker := time.NewTicker(ms.reloadCycle)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			newConfig := ms.loadExternalConfig()
			ms.processConfigUpdate(newConfig)
		case <-ms.stopChan:
			return
		}
	}
}

// 加载外部配置（示例实现）
func (ms *MetaServer) loadExternalConfig() *Config {
	// 实际项目应实现从数据库/配置中心获取
	// 此处使用模拟数据
	return &Config{
		num:     time.Now().Second()%100 + 50, // 模拟变化
		version: ms.cache.active.version + 1,
	}
}

// 处理配置更新
func (ms *MetaServer) processConfigUpdate(newConfig *Config) {
	current := ms.cache.getActiveConfig()

	if newConfig.num != current.num {
		fmt.Printf("Config changed from %d to %d\n", current.num, newConfig.num)

		// 原子更新配置
		ms.cache.mu.Lock()
		ms.cache.fallback = ms.cache.active
		ms.cache.active = newConfig
		ms.cache.mu.Unlock()

		// 生成新策略
		newStrategy := ms.generateStrategy(newConfig)
		ms.cache.strategy = newStrategy

		// 通知关联组件
		select {
		//还没有用
		case ms.configChan <- newConfig:
		default:
			fmt.Println("Config channel full, drop update")
		}
	}
}

// 安全停止服务
func (ms *MetaServer) Shutdown(ctx context.Context) error {
	close(ms.stopChan)
	_ = ms.listener.Close()

	// 等待协程退出
	select {
	case <-time.After(3 * time.Second):
		return fmt.Errorf("shutdown timeout")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func main() {
	metaserver := NewMetaServer("8096", 60*time.Second)
	timer := time.NewTimer(1 * time.Hour)
	select {
	case <-timer.C:
		{
			log.Println("一小时自动停机")
			metaserver.Shutdown(context.Background())
		}
	}
}
