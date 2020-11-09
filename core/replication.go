package core

import (
	"context"
	"github.com/go-redis/redis/v8"
	"log"
	"math"
	"math/rand"
	"time"
)

type Replicator struct {
	server *redis.Client
	addr   string
	port   int

	cancelFn func()
}

func NewReplicator(rdb *redis.Client, addr string, port int) *Replicator {
	return &Replicator{
		server:   rdb,
		addr:     addr,
		port:     port,
		cancelFn: nil,
	}
}

func (r *Replicator) BeginFollow(ctx context.Context) {
	if r.cancelFn != nil {
		r.cancelFn()
	}

	ncx, cancel := context.WithCancel(ctx)
	r.cancelFn = cancel
	go r.follow(ncx)
}

func (r *Replicator) StopFollow(ctx context.Context) {
	if r.cancelFn != nil {
		r.cancelFn()
	}

	ncx, cancel := context.WithCancel(ctx)
	r.cancelFn = cancel
	go r.unfollow(ncx)
}

func (r *Replicator) follow(ctx context.Context) {
	log.Println("starting data replication")
	cmd := redis.NewStatusCmd(ctx, "follow", r.addr, r.port)
	processCmd(ctx, r.server, cmd)
}

func (r *Replicator) unfollow(ctx context.Context) {
	log.Println("stopping data replication")
	cmd := redis.NewStatusCmd(ctx, "follow", "no", "one")
	processCmd(ctx, r.server, cmd)
}

func processCmd(ctx context.Context, client *redis.Client, cmd *redis.StatusCmd) {
	attempt := 1
	delay := time.Duration(0)

	for {
		select {
		case <-ctx.Done():
			return

		case <-time.After(delay):
			err := client.Process(ctx, cmd)
			if err == nil {
				return
			}

			attempt++
			delay = expDelay(attempt, 10*time.Second)
			log.Printf("failed to update replication upstream. attempt %d. error: %s", attempt, err)
		}
	}
}

func expDelay(attempt int, cap time.Duration) time.Duration {
	exp := math.Pow(2, float64(attempt))
	n := time.Duration((exp - 1) / 2)
	jitter := time.Duration(rand.Intn(500) + 100)

	delay := (n * time.Second) + (jitter * time.Millisecond)
	if delay > cap {
		return cap
	}

	return delay
}
