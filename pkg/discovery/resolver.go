package discovery

import (
	"context"
	"fmt"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	"google.golang.org/grpc/resolver"
)

const schema = "etcd"

// Resolver for grpc client
type Resolver struct {
	schema      string
	cli         *clientv3.Client
	closeCh     chan struct{}
	cc          resolver.ClientConn
	srvAddrList []resolver.Address
}

// NewResolver create a new etcd resolver builder
func NewResolver(endpoints []string) resolver.Builder {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		zap.L().Fatal("etcd client connection failed", zap.Error(err))
	}

	return &Resolver{
		schema:  schema,
		cli:     cli,
		closeCh: make(chan struct{}),
	}
}

// Build creates a new resolver for the given target
func (r *Resolver) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	r.cc = cc

	prefix := "/" + target.URL.Host
	if err := r.sync(prefix); err != nil {
		return nil, err
	}

	go r.watch(prefix)

	return r, nil
}

func (r *Resolver) Scheme() string {
	return r.schema
}

func (r *Resolver) ResolveNow(rn resolver.ResolveNowOptions) {}

func (r *Resolver) Close() {
	close(r.closeCh)
	r.cli.Close()
}

func (r *Resolver) sync(prefix string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := r.cli.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return err
	}

	var addrList []resolver.Address
	for _, kv := range resp.Kvs {
		addrList = append(addrList, resolver.Address{Addr: extractAddr(string(kv.Key))})
	}
	r.srvAddrList = addrList
	return r.cc.UpdateState(resolver.State{Addresses: r.srvAddrList})
}

func (r *Resolver) watch(prefix string) {
	watchCh := r.cli.Watch(context.Background(), prefix, clientv3.WithPrefix())

	for {
		select {
		case <-r.closeCh:
			return
		case resp := <-watchCh:
			for _, ev := range resp.Events {
				addr := extractAddr(string(ev.Kv.Key))
				switch ev.Type {
				case clientv3.EventTypePut:
					if !r.exist(addr) {
						r.srvAddrList = append(r.srvAddrList, resolver.Address{Addr: addr})
						r.cc.UpdateState(resolver.State{Addresses: r.srvAddrList})
					}
				case clientv3.EventTypeDelete:
					if s, ok := r.remove(addr); ok {
						r.srvAddrList = s
						r.cc.UpdateState(resolver.State{Addresses: r.srvAddrList})
					}
				}
			}
		}
	}
}

func extractAddr(key string) string {
	idx := strings.LastIndex(key, "/")
	if idx == -1 {
		return key
	}
	return key[idx+1:]
}

func (r *Resolver) exist(addr string) bool {
	for _, a := range r.srvAddrList {
		if a.Addr == addr {
			return true
		}
	}
	return false
}

func (r *Resolver) remove(addr string) ([]resolver.Address, bool) {
	for i, a := range r.srvAddrList {
		if a.Addr == addr {
			return append(r.srvAddrList[:i], r.srvAddrList[i+1:]...), true
		}
	}
	return r.srvAddrList, false
}

// BuildDialTarget returns a string in the format "etcd:///{serviceName}"
func BuildDialTarget(serviceName string) string {
	return fmt.Sprintf("%s:///%s", schema, serviceName)
}
