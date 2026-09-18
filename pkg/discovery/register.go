package discovery

import (
	"context"
	"fmt"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/naming/endpoints"
	"go.uber.org/zap"
)

// ServerInfo 服务实例元数据
type ServerInfo struct {
	Name   string
	Addr   string
	Weight int
}

// Register etcd 官方服务注册管理器
type Register struct {
	cli         *clientv3.Client
	manager     endpoints.Manager
	leaseID     clientv3.LeaseID
	endpointKey string
	closeCh     chan struct{}
	info        ServerInfo
}

// NewRegister 基于 etcd 官方 endpoints.Manager 创建服务注册器
func NewRegister(endpointsList []string, info ServerInfo, ttl int64) (*Register, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpointsList,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	em, err := endpoints.NewManager(cli, info.Name)
	if err != nil {
		_ = cli.Close()
		return nil, err
	}

	reg := &Register{
		cli:         cli,
		manager:     em,
		info:        info,
		endpointKey: fmt.Sprintf("%s/%s", info.Name, info.Addr),
		closeCh:     make(chan struct{}),
	}

	if err := reg.register(ttl); err != nil {
		_ = cli.Close()
		return nil, err
	}

	return reg, nil
}

func (r *Register) register(ttl int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	leaseResp, err := r.cli.Grant(ctx, ttl)
	if err != nil {
		return err
	}
	r.leaseID = leaseResp.ID

	keepAliveCh, err := r.cli.KeepAlive(context.Background(), r.leaseID)
	if err != nil {
		return err
	}

	ep := endpoints.Endpoint{
		Addr:     r.info.Addr,
		Metadata: r.info.Weight,
	}

	if err := r.manager.AddEndpoint(ctx, r.endpointKey, ep, clientv3.WithLease(r.leaseID)); err != nil {
		return err
	}

	go r.listenLeaseResp(keepAliveCh)
	return nil
}

func (r *Register) listenLeaseResp(keepAliveCh <-chan *clientv3.LeaseKeepAliveResponse) {
	for {
		select {
		case <-r.closeCh:
			return
		case leaseKeepResp, ok := <-keepAliveCh:
			if !ok || leaseKeepResp == nil {
				zap.L().Warn("etcd lease keepalive closed", zap.String("service", r.info.Name), zap.String("addr", r.info.Addr))
				return
			}
		}
	}
}

// Stop 优雅下线服务：先从官方 endpoints 移除该实例，再销毁租约与连接
func (r *Register) Stop() {
	close(r.closeCh)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if r.manager != nil && r.endpointKey != "" {
		if err := r.manager.DeleteEndpoint(ctx, r.endpointKey); err != nil {
			zap.L().Error("delete endpoint failed", zap.String("key", r.endpointKey), zap.Error(err))
		}
	}

	if r.leaseID != 0 {
		if _, err := r.cli.Revoke(ctx, r.leaseID); err != nil {
			zap.L().Error("revoke lease failed", zap.Error(err))
		}
	}
	_ = r.cli.Close()
}

// BuildRegPath 保留辅助方法兼容性
func (r *Register) BuildRegPath(info ServerInfo) string {
	return fmt.Sprintf("/%s/%s", info.Name, info.Addr)
}
