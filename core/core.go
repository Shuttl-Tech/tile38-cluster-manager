package core

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-go/statsd"
	"github.com/go-redis/redis/v8"
	"github.com/hashicorp/consul/api"
	"github.com/urfave/cli/v2"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type Manager struct {
	config *Config
	rdb    *redis.Client
	consul *api.Client
	stats  *statsd.Client

	rx     *sync.RWMutex
	role   Role
	leader string

	mx         *sync.Mutex
	membership *sync.Cond

	roleReady chan struct{}

	bridgeAddr string
	bridgePort int
}

func NewManager(config *Config) (*Manager, error) {
	opts := &redis.Options{
		Addr:            fmt.Sprintf("%s:%d", config.ServerAddr, config.ServerPort),
		DialTimeout:     time.Second,
		ReadTimeout:     time.Second,
		WriteTimeout:    time.Second,
		MaxRetries:      10,
		MinRetryBackoff: time.Second,
		MaxRetryBackoff: 5 * time.Second,
	}

	rdb := redis.NewClient(opts)
	_, err := rdb.Ping(context.Background()).Result()
	if err != nil {
		return nil, fmt.Errorf("server connection is not available. %s", err)
	}

	conf := &api.Config{
		Address: config.ConsulAddr,
	}

	consul, err := api.NewClient(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize consul client. %s", err)
	}

	_, err = consul.Status().Leader()
	if err != nil {
		return nil, fmt.Errorf("failed to query for consul leader. %s", err)
	}

	statsdAddr := fmt.Sprintf("%s:%s", config.DogstatsdAddr, config.DogstatsdPort)
	stats, err := statsd.New(statsdAddr, statsd.WithNamespace("tile38.cluster"))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize statsd client. %s", err)
	}

	bridgeAddr, bridgePort, err := freeLocalAddr()
	if err != nil {
		return nil, fmt.Errorf("failed to find a free network port for replication bridge. %s", err)
	}

	mx := new(sync.Mutex)

	return &Manager{
		config:     config,
		rdb:        rdb,
		consul:     consul,
		stats:      stats,
		rx:         new(sync.RWMutex),
		role:       RoleFollower,
		mx:         mx,
		membership: sync.NewCond(mx),
		roleReady:  make(chan struct{}),
		bridgeAddr: bridgeAddr,
		bridgePort: bridgePort,
	}, nil
}

// Handler returns the handler function for *cli.App
func Handler(config *Config) func(c *cli.Context) error {
	return func(c *cli.Context) error {
		manager, err := NewManager(config)
		if err != nil {
			return fmt.Errorf("failed to initialize cluster manager. %s", err)
		}

		return Start(c.Context, config, manager)
	}
}

func Start(parent context.Context, config *Config, manager *Manager) error {
	log.Printf("booting up with ID %s", config.ID)

	// stopped and stopLeader channels coordinate leadership
	// transfer during shutdown. We want to ensure a smooth transition
	// of leadership as much as possible. For that, when we receive
	// a signal to stop, we give up leadership and stop the role
	// manager loop before everything else.
	// stopLeader channel is closed when the signal is received, and
	// a signal is awaited on stopped channel to indicate that the loop
	// has stopped.
	// When a signal is received back on stopped channel, it means we can
	// proceed to shut down all the other threads.
	stopped := make(chan struct{})
	ctx, cancel, notif := makeContext(parent)
	stopLeader := handleSig(notif, stopped, cancel)

	threadCtx := NewThreadContext(ctx)
	defer threadCtx.Wait()

	sc := make(chan struct{})
	go func() {
		close(sc)
		manager.WaitForRoleChange()
		close(manager.roleReady)
	}()

	leaders := make(chan Peers)
	followers := make(chan Peers)
	forcer := &forcedUpdate{
		change:   manager.RoleChanged(ctx),
		leader:   make(chan struct{}, 1),
		follower: make(chan struct{}, 1),
	}

	<-sc
	go manager.StartMetricsFunnel(threadCtx)
	go manager.ManageMembership(threadCtx, config.ManageServiceRegistration, stopLeader, stopped)
	go forcer.WatchAndForce(threadCtx)
	go manager.WatchPeers(threadCtx, leaders, followers, forcer)

	return manager.StartProxy(threadCtx, leaders, followers)
}

func handleSig(sig <-chan struct{}, stopped <-chan struct{}, cancel context.CancelFunc) <-chan struct{} {
	stopChan := make(chan struct{})

	go func() {
		<-sig
		close(stopChan)
		<-stopped
		cancel()
	}()

	return stopChan
}

// makeContext creates a cacellable context and returns the context, its cancel
// func, and a channel that is closed on SIGINT and SIGTERM.
// The caller is expected to close the context. if another signal is received
// after the channel is closed, the process will exit forcefully with exit code 1.
func makeContext(c context.Context) (context.Context, context.CancelFunc, <-chan struct{}) {
	exit := make(chan os.Signal, 1)
	notif := make(chan struct{})

	signal.Notify(exit, os.Interrupt, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(c)

	go func() {
		select {
		case <-c.Done():
		case <-exit:
		}

		log.Println("attempting to shut down gracefully...")
		close(notif)

		// A second signal will cause the manager to exit forcefully
		<-exit
		log.Println("exiting forcefully...")
		os.Exit(1)
	}()

	return ctx, cancel, notif
}
