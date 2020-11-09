package core

import (
	"context"
	"github.com/go-redis/redis/v8"
	"github.com/hashicorp/consul/connect"
	"net"
	"sync"
	"time"
)

type connectDialFunc func(context.Context, connect.Resolver) (net.Conn, error)

type TestContext struct {
	context.Context
	cancel context.CancelFunc
	wg     *sync.WaitGroup

	tile38Bin    string
	consulBin    string
	bindPortBase int
	clusterCount int
}

type instance struct {
	id         string
	manager    *Manager
	shutdown   context.CancelFunc
	underlying *redis.Client
	leader     *redis.Client
	follower   *redis.Client
}

type instances []*instance

func (i instances) shutdown() {
	for _, ins := range i {
		ins.shutdown()
	}
}

func (i instances) leader(ctx context.Context, attempts int) (int, *instance) {
	for idx, ins := range i {
		if ins.manager.Role().Role == RoleLeader {
			return idx, ins
		}
	}

	if attempts == 0 {
		return 0, nil
	}

	d := 3 * time.Second
	itr := 0

	for {
		select {
		case <-ctx.Done():
			panic("context canceled before a leader was found")
		case <-time.After(d):
			for idx, ins := range i {
				if ins.manager.Role().Role == RoleLeader {
					return idx, ins
				}
			}

			if itr == attempts {
				return 0, nil
			}

			itr++
		}
	}
}
