package discovery

import (
	"fmt"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	etcdresolver "go.etcd.io/etcd/client/v3/naming/resolver"
	gresolver "google.golang.org/grpc/resolver"
)

// NewResolver 创建基于 etcd 官方命名解析器的 gRPC Resolver Builder
func NewResolver(endpoints []string) (gresolver.Builder, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	return etcdresolver.NewBuilder(cli)
}

// BuildDialTarget 生成符合 gRPC 与 etcd 官方 resolver 规范的 Dial 目标：etcd:///{serviceName}
func BuildDialTarget(serviceName string) string {
	return fmt.Sprintf("etcd:///%s", serviceName)
}
