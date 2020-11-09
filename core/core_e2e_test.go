package core

import (
	"context"
	"github.com/go-redis/redis/v8"
	"github.com/hashicorp/consul/connect"
	"os"
	"sync"
	"testing"
	"time"
)

func TestAccCoreEndToEnd(t *testing.T) {
	if os.Getenv("TEST_ACC") == "" {
		t.Skipf("Skipping E2E test. To execute this test, set 'TEST_ACC' environment variable")
	}

	ctx := &TestContext{
		wg:           new(sync.WaitGroup),
		tile38Bin:    "tile38-server",
		consulBin:    "consul",
		bindPortBase: 9000,
		clusterCount: 3,
	}

	deadline, hasTestDeadline := t.Deadline()
	if hasTestDeadline {
		ctx.Context, ctx.cancel = context.WithDeadline(context.Background(), deadline)
	} else {
		ctx.Context, ctx.cancel = context.WithTimeout(context.Background(), 5*time.Second)
	}

	t.Run("End to End Core Test", EndToEndClusterTest(ctx))
}

func EndToEndClusterTest(ctx *TestContext) func(t *testing.T) {
	return func(t *testing.T) {
		if err := preTestChecks(ctx); err != nil {
			t.Fatalf("pre test checks failed with error. %s", err)
		}

		defer ctx.wg.Wait()
		defer ctx.cancel()

		consul, dereg, err := startConsul(ctx, t)
		if err != nil {
			t.Errorf("failed to start consul. %s", err)
			return
		}
		defer dereg()

		err = startMetricsSink(ctx, t)
		if err != nil {
			t.Errorf("failed to start metrics sink. %s", err)
			return
		}

		svc, err := connect.NewService(testClientServiceName, consul)
		if err != nil {
			t.Errorf("failed to get connect service for client. %s", err)
			return
		}
		defer svc.Close()
		<-svc.ReadyWait()

		cluster, err := startClusters(ctx, t, svc.Dial)
		if err != nil {
			t.Errorf("failed to start cluster. %s", err)
			return
		}
		defer cluster.shutdown()

		for _, ins := range cluster {
			<-ins.manager.roleReady
		}

		t.Run("write to all instances", testWriteToAllInstances(ctx, "initial-test", cluster))
		t.Run("read from all instances", testReadKnownKeysFromAllInstances(ctx, "initial-test", cluster))
		t.Run("test resiliency to leader loss", testLeaderLossResilience(ctx, &cluster))
		t.Run("test new node replicates all data", testNewMemberJoin(ctx, &cluster, svc.Dial))
	}
}

func testNewMemberJoin(ctx *TestContext, cluster *instances, dialer connectDialFunc) func(*testing.T) {
	return func(t *testing.T) {
		var oldIds []string
		for _, ins := range *cluster {
			oldIds = append(oldIds, ins.id)
		}

		newInstance, err := startCluster(ctx, t, len(*cluster)+2, dialer)
		if err != nil {
			t.Errorf("failed to start a new instance. %s", err)
			return
		}

		*cluster = append(*cluster, newInstance)
		<-newInstance.manager.roleReady
		newRole := newInstance.manager.Role().Role

		if newRole != RoleFollower {
			t.Errorf("expected the new member to be a follower. found role %s", newRole.String())
			return
		}

		// give it time to replicate data.
		// TODO: There is no science here. We know that the new member
		//       has joined the cluster and is a follower. Replication time
		//       is a function of data volume, and given that we only ever
		//       write about 10 records in the entire suite, it shouldn't take
		//       more than 3 seconds to replicate the full set.
		//       We might find a better way to check for replication before
		//       proceeding with the tests, but for now that doesn't seem
		//       necessary.
		time.Sleep(3 * time.Second)

		expectedData := map[string]string{
			"initial-testinstance-0": "initial-testinstance-0",
			"initial-testinstance-1": "initial-testinstance-1",
			"initial-testinstance-2": "initial-testinstance-2",
		}

		for _, id := range oldIds {
			expectedData["leader-loss-test"+id] = "leader-loss-test" + id
		}

		// Check for old data on all running instances
		t.Run("read initial data from all instances", testReadKeysFromAllInstances(ctx, *cluster, expectedData))
		t.Run("read leader loss data from all instances", testReadKeysFromAllInstances(ctx, *cluster, expectedData))

		// Ensure that all instances can read and write after the new member joined
		t.Run("write new member data to all instances", testWriteToAllInstances(ctx, "new-member-joined-test", *cluster))
		t.Run("read new member data from all instances", testReadKnownKeysFromAllInstances(ctx, "new-member-joined-test", *cluster))
	}
}

func testLeaderLossResilience(ctx *TestContext, cluster *instances) func(*testing.T) {
	return func(t *testing.T) {
		idx, leader := cluster.leader(ctx, 0)
		if leader == nil {
			t.Errorf("failed to find the cluster leader")
			return
		}

		leader.shutdown()
		*cluster = append((*cluster)[:idx], (*cluster)[idx+1:]...)

		_, newLeader := cluster.leader(ctx, 10)
		if newLeader == nil {
			t.Errorf("failed to find the cluster leader in 10 attempts")
			return
		}

		// cluster takes a while to settle down after leader loss.
		// TODO: Again, there is no science here. The lock wait time
		//       on leadership lock is 15 seconds, so the worst case
		//       scenario should only take about 15+3 seconds.
		time.Sleep(20 * time.Second)

		t.Run("write to all instances", testWriteToAllInstances(ctx, "leader-loss-test", *cluster))
		t.Run("read from all instances", testReadKnownKeysFromAllInstances(ctx, "leader-loss-test", *cluster))
	}
}

func testWriteToAllInstances(ctx *TestContext, prefix string, cluster instances) func(*testing.T) {
	return func(t *testing.T) {
		for _, ins := range cluster {
			cmd := redis.NewStatusCmd(ctx, "set", "test-data", prefix+ins.id, "string", prefix+ins.id)
			err := ins.leader.Process(ctx, cmd)
			if err != nil {
				t.Errorf("failed to write data to %s, error: %s", ins.id, err)
			}
		}
	}
}

func testReadKeysFromAllInstances(ctx *TestContext, cluster instances, data map[string]string) func(*testing.T) {
	return func(t *testing.T) {
		for _, ins := range cluster {
			for k, v := range data {
				cmd := redis.NewStringCmd(ctx, "get", "test-data", k)
				_ = ins.follower.Process(ctx, cmd)
				result, err := cmd.Result()
				if err != nil {
					t.Errorf("failed to read data from %s. error: %s", k, err)
				}

				if result != v {
					t.Errorf("unexpected value read from %s. expected %q, got %q", k, v, result)
				}
			}
		}
	}
}

func testReadKnownKeysFromAllInstances(ctx *TestContext, prefix string, cluster instances) func(*testing.T) {
	return func(t *testing.T) {
		var ids []string
		for _, ins := range cluster {
			ids = append(ids, ins.id)
		}

		for _, ins := range cluster {
			for _, id := range ids {
				cmd := redis.NewStringCmd(ctx, "get", "test-data", prefix+id)
				_ = ins.follower.Process(ctx, cmd)
				result, err := cmd.Result()
				if err != nil {
					t.Errorf("failed to read data from %s. error: %s", id, err)
				}

				expect := prefix + id
				if result != expect {
					t.Errorf("unexpected value read from %s. expected %q, got %q", id, expect, result)
				}
			}
		}
	}
}
