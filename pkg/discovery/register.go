package discovery

import (
	"context"
	"fmt"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
)

// Register for etcd
type Register struct {
	cli         *clientv3.Client
	leaseID     clientv3.LeaseID
	keepAliveCh <-chan *clientv3.LeaseKeepAliveResponse
	info        ServerInfo
	closeCh     chan struct{}
}

type ServerInfo struct {
	Name    string
	Addr    string
	Weight  int
}

// NewRegister create a register based on etcd
func NewRegister(endpoints []string, info ServerInfo, ttl int64) (*Register, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	reg := &Register{
		cli:     cli,
		info:    info,
		closeCh: make(chan struct{}),
	}

	if err := reg.register(ttl); err != nil {
		return nil, err
	}

	return reg, nil
}

func (r *Register) register(ttl int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()

	leaseResp, err := r.cli.Grant(ctx, ttl)
	if err != nil {
		return err
	}
	r.leaseID = leaseResp.ID

	if r.keepAliveCh, err = r.cli.KeepAlive(context.Background(), r.leaseID); err != nil {
		return err
	}

	data := fmt.Sprintf(`{"Addr":"%s","Weight":%d}`, r.info.Addr, r.info.Weight)
	_, err = r.cli.Put(ctx, r.BuildRegPath(r.info), data, clientv3.WithLease(r.leaseID))
	if err != nil {
		return err
	}

	go r.listenLeaseResp()
	return nil
}

func (r *Register) listenLeaseResp() {
	for {
		select {
		case <-r.closeCh:
			return
		case leaseKeepResp := <-r.keepAliveCh:
			if leaseKeepResp == nil {
				zap.L().Warn("lease closed")
				return
			}
		}
	}
}

func (r *Register) Stop() {
	close(r.closeCh)
	if _, err := r.cli.Revoke(context.Background(), r.leaseID); err != nil {
		zap.L().Error("revoke lease failed", zap.Error(err))
	}
	r.cli.Close()
}

func (r *Register) BuildRegPath(info ServerInfo) string {
	return fmt.Sprintf("/%s/%s", info.Name, info.Addr)
}
