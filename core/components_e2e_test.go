package core

import (
	"context"
	"fmt"
	"github.com/go-redis/redis/v8"
	connectagent "github.com/hashicorp/consul/agent/connect"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/connect"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const (
	testClientServiceName = "test-client-service"
	leaderServiceName     = "tile38-leader"
	followerServiceName   = "tile38-follower"
	defaultListenAddr     = "127.0.0.1"
)

func preTestChecks(ctx *TestContext) error {
	_, err := exec.LookPath(ctx.tile38Bin)
	if err != nil {
		return err
	}

	_, err = exec.LookPath(ctx.consulBin)
	if err != nil {
		return err
	}

	return nil
}

func startCluster(ctx *TestContext, t *testing.T, index int, dialer connectDialFunc) (*instance, error) {
	sp := ctx.bindPortBase + 100 + index
	lp := ctx.bindPortBase + 200 + index
	fp := ctx.bindPortBase + 300 + index

	config := &Config{
		ID:                        fmt.Sprintf("instance-%d", index),
		Advertise:                 defaultListenAddr,
		ServerAddr:                defaultListenAddr,
		ServerPort:                sp,
		LeaderBindAddr:            defaultListenAddr,
		LeaderBindPort:            lp,
		FollowerBindAddr:          defaultListenAddr,
		FollowerBindPort:          fp,
		ConsulAddr:                defaultListenAddr + ":8500",
		ConsulLockPrefix:          "_locks/cluster",
		DogstatsdAddr:             defaultListenAddr,
		DogstatsdPort:             fmt.Sprintf("%d", ctx.bindPortBase+1000),
		ManageServiceRegistration: true,
		LeaderServiceName:         leaderServiceName,
		FollowerServiceName:       followerServiceName,
	}

	tileCancel, err := startTile38(ctx, t, sp)
	if err != nil {
		return nil, err
	}

	managerCtx, managerCancel := context.WithCancel(ctx)
	m, err := NewManager(config)
	if err != nil {
		return nil, err
	}

	ctx.wg.Add(1)
	go func(c context.Context, conf *Config, manager *Manager) {
		defer ctx.wg.Done()
		err := Start(c, conf, manager)
		if err != nil {
			t.Errorf("cluster manager failed with error. %s", err)
		}
	}(managerCtx, config, m)

	shutdown := func() {
		managerCancel()
		tileCancel()
	}

	return &instance{
		id:         config.ID,
		manager:    m,
		shutdown:   shutdown,
		underlying: redisClient("", defaultListenAddr, sp, nil),
		leader:     redisClient(leaderServiceName, defaultListenAddr, lp, dialer),
		follower:   redisClient(followerServiceName, defaultListenAddr, fp, dialer),
	}, nil
}

func startClusters(ctx *TestContext, t *testing.T, dialer connectDialFunc) (instances, error) {
	var result instances
	for i := 0; i < ctx.clusterCount; i++ {
		inst, err := startCluster(ctx, t, i, dialer)
		if err != nil {
			return nil, err
		}
		result = append(result, inst)
	}

	return result, nil
}

func redisClient(svcName, addr string, port int, dialer connectDialFunc) *redis.Client {
	opts := &redis.Options{
		Addr:            fmt.Sprintf("%s:%d", addr, port),
		DialTimeout:     time.Second,
		ReadTimeout:     time.Second,
		WriteTimeout:    time.Second,
		MaxRetries:      2,
		MinRetryBackoff: 500 * time.Millisecond,
		MaxRetryBackoff: 2 * time.Second,
		Dialer: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if dialer == nil {
				return net.Dial(network, addr)
			}

			return dialer(ctx, &connect.StaticResolver{
				Addr: addr,
				CertURI: &connectagent.SpiffeIDService{
					Namespace:  "default",
					Datacenter: "dc1",
					Service:    svcName,
				},
			})
		},
	}

	return redis.NewClient(opts)
}

func startTile38(ctx *TestContext, t *testing.T, port int) (context.CancelFunc, error) {
	cmdCtx, cmdCancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(cmdCtx, ctx.tile38Bin, "-p", fmt.Sprintf("%d", port))
	cmd.Dir = t.TempDir()

	if os.Getenv("TEST_LOG_TILE38") == "true" {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	ctx.wg.Add(1)
	go func() {
		_ = cmd.Wait()
		ctx.wg.Done()
	}()

	return cmdCancel, nil
}

func startConsul(ctx *TestContext, t *testing.T) (*api.Client, func(), error) {
	ctx.wg.Add(1)

	cmd := exec.CommandContext(ctx, ctx.consulBin, "agent", "-dev")

	if os.Getenv("TEST_LOG_CONSUL") == "true" {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}

	go func() {
		_ = cmd.Wait()
		ctx.wg.Done()
	}()

	client, err := api.NewClient(&api.Config{
		Address: "127.0.0.1:8500",
		Scheme:  "http",
	})

	if err != nil {
		t.Errorf("failed to create consul client. %s", err)
		_ = cmd.Process.Kill()
		return nil, nil, err
	}

	// Wait for consul to come online and start accepting connections
	for {
		_, err := client.Status().Leader()
		if err != nil {
			t.Logf("waiting for consul to come online. %s", err)
			time.Sleep(2 * time.Second)
		} else {
			break
		}
	}

	// register a test service in consul catalog for tls interaction
	testSvc := &api.AgentServiceRegistration{
		ID:      testClientServiceName,
		Name:    testClientServiceName,
		Port:    5678,
		Address: "127.0.0.1",
		Connect: &api.AgentServiceConnect{
			Native: true,
		},
	}

	err = client.Agent().ServiceRegisterOpts(testSvc, api.ServiceRegisterOpts{})
	if err != nil {
		t.Errorf("failed to register test client service. %s", err)
		_ = cmd.Process.Kill()
		return nil, nil, err
	}

	dereg := func() {
		err := client.Agent().ServiceDeregister(testClientServiceName)
		if err != nil {
			t.Errorf("failed to deregister service from consul catalog. %s", err)
		}
	}

	return client, dereg, nil
}

func startMetricsSink(ctx *TestContext, t *testing.T) error {
	addr := &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: ctx.bindPortBase + 1000,
	}

	l, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Errorf("failed to listen on UDP interface. %s", err)
		return err
	}

	go func() {
		<-ctx.Done()
		err := l.Close()
		if err != nil {
			t.Errorf("failed to close UDP listener. %s", err)
		}
	}()

	_ = l.SetReadBuffer(512)
	go func() {
		for {
			data := make([]byte, 512)
			_, err := l.Read(data)
			if err != nil {
				if strings.Contains(err.Error(), "use of closed network connection") {
					return
				}

				t.Logf("failed to read from UDP listener. %s", err)
			}
		}
	}()

	return nil
}
