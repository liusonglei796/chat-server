package cdc

import (
	"context"
	"fmt"

	"github.com/go-mysql-org/go-mysql/canal"
	"github.com/go-mysql-org/go-mysql/mysql"
	"go.uber.org/zap"
)

// SyncerConfig CDC 同步器配置
type SyncerConfig struct {
	Addr              string        // MySQL 地址 host:port
	User              string        // 用户名 (具备 REPLICATION SLAVE, SELECT 权限)
	Password          string        // 密码
	ServerID          uint32        // 唯一 slave server id (不能与主库 server-id 冲突)
	IncludeTableRegex []string      // 监听表正则表达式，如 []string{"^chat_.*\\.outbox$"}
	GTIDStore         GTIDStore     // GTID 持久化存储
	PublishFn         PublishFunc   // Kafka 投递函数
}

// CDCSyncer 封装基于 go-mysql/canal 的增量 Binlog 同步器
type CDCSyncer struct {
	canal     *canal.Canal
	gtidStore GTIDStore
	handler   *OutboxEventHandler
}

// NewCDCSyncer 创建 CDCSyncer 实例
func NewCDCSyncer(cfg SyncerConfig) (*CDCSyncer, error) {
	canalCfg := canal.NewDefaultConfig()
	canalCfg.Addr = cfg.Addr
	canalCfg.User = cfg.User
	canalCfg.Password = cfg.Password
	canalCfg.Flavor = "mysql"

	if cfg.ServerID != 0 {
		canalCfg.ServerID = cfg.ServerID
	} else {
		canalCfg.ServerID = 1001
	}

	// 禁用 mysqldump，仅订阅 binlog 增量数据流
	canalCfg.Dump.ExecutionPath = ""

	if len(cfg.IncludeTableRegex) > 0 {
		canalCfg.IncludeTableRegex = cfg.IncludeTableRegex
	} else {
		canalCfg.IncludeTableRegex = []string{`^chat_.*\.outbox$`}
	}

	c, err := canal.NewCanal(canalCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize canal: %w", err)
	}

	handler := NewOutboxEventHandler(cfg.GTIDStore, cfg.PublishFn)
	c.SetEventHandler(handler)

	return &CDCSyncer{
		canal:     c,
		gtidStore: cfg.GTIDStore,
		handler:   handler,
	}, nil
}

// Start 启动 CDC 监听流
func (s *CDCSyncer) Start(ctx context.Context) error {
	var startGTID mysql.GTIDSet

	if s.gtidStore != nil {
		set, err := s.gtidStore.Get(ctx)
		if err != nil {
			zap.L().Warn("failed to get gtid from store, falling back to master gtid", zap.Error(err))
		} else {
			startGTID = set
		}
	}

	// 若未找到持久化位点，则从当前主库最新 GTID 集合开始监听（避免回溯启动前产生的大量旧日志）
	if startGTID == nil || startGTID.IsEmpty() {
		masterGTID, err := s.canal.GetMasterGTIDSet()
		if err != nil {
			return fmt.Errorf("failed to fetch current master GTID set: %w", err)
		}
		startGTID = masterGTID
		zap.L().Info("starting CDC syncer from current master GTID set",
			zap.String("gtid", startGTID.String()),
		)
		if s.gtidStore != nil {
			_ = s.gtidStore.Save(ctx, startGTID)
		}
	} else {
		zap.L().Info("resuming CDC syncer from saved GTID set",
			zap.String("gtid", startGTID.String()),
		)
	}

	errChan := make(chan error, 1)
	go func() {
		if err := s.canal.StartFromGTID(startGTID); err != nil {
			errChan <- err
		}
	}()

	select {
	case <-ctx.Done():
		zap.L().Info("closing CDC syncer gracefully...")
		s.canal.Close()
		return nil
	case err := <-errChan:
		return fmt.Errorf("canal runtime error: %w", err)
	}
}

// Close 手动停止同步器
func (s *CDCSyncer) Close() {
	if s.canal != nil {
		s.canal.Close()
	}
}
