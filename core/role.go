package core

import (
	"context"
	"fmt"
	"github.com/hashicorp/consul/api"
	"log"
	"path"
	"time"
)

type Role int

const (
	RoleFollower Role = iota + 1
	RoleLeader
)

type lockBlockStatus int

const (
	lockBlockStatusContinue lockBlockStatus = iota + 1
	lockBlockStatusStop
)

const lockWaitTime = 15 * time.Second

func (r Role) String() string {
	switch r {
	case RoleLeader:
		return "leader"
	case RoleFollower:
		return "follower"
	default:
		return fmt.Sprintf("Role(%d)", r)
	}
}

type Membership struct {
	Role   Role
	Leader string
}

func (m *Manager) Role() Membership {
	m.rx.RLock()
	defer m.rx.RUnlock()
	return Membership{
		Role:   m.role,
		Leader: m.leader,
	}
}

func (m *Manager) SetRole(r Role, leader string) {
	m.rx.Lock()
	defer m.rx.Unlock()

	if m.role == r && m.leader == leader {
		return
	}

	log.Printf("current role is %s, cluster leader is %s", r.String(), leader)

	m.role = r
	m.leader = leader

	m.membership.Broadcast()
}

func (m *Manager) WaitForRoleChange() Membership {
	m.mx.Lock()
	defer m.mx.Unlock()
	m.membership.Wait()
	return m.Role()
}

func (m *Manager) RoleChanged(ctx context.Context) <-chan Membership {
	c := make(chan Membership)

	go func() {
		c <- m.Role()

		for {
			select {
			case c <- m.WaitForRoleChange():
				// noop
			case <-ctx.Done():
				return
			}
		}
	}()

	return c
}

func (m *Manager) handleRegistration(ctx context.Context) (func(), error) {
	deregister := func() {
		err := m.DeregisterServices()
		if err != nil {
			log.Printf("failed to deregister services from consul catalog. %s", err)
		}
	}

	attempt := 0
	delay := time.Duration(0)

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled")

		case <-time.After(delay):
			err := m.RegisterServices()
			if err == nil {
				return deregister, nil
			}

			log.Printf("failed to register services in consul catalog. attempt %d. error %s", attempt, err)
			delay = expDelay(attempt, 5*time.Second)
		}
	}
}

func (m *Manager) ManageMembership(ctx *Context, registerSelf bool, stopChan <-chan struct{}, stoppedSig chan<- struct{}) {
	ctx.Begin()
	defer ctx.End()
	defer func() {
		close(stoppedSig)
	}()

	if registerSelf {
		deregister, err := m.handleRegistration(ctx)
		if err != nil {
			log.Printf("failed to register services in catalog. %s", err)
			return
		}

		defer deregister()
	}

	log.Printf("Manager %s has entered candidate state", m.config.ID)

	lock := m.createLeadershipLock()

	for {
		select {
		case <-stopChan:
			log.Println("stopping the membership manager loop")
			return
		default:
		}

		handle, err := lock.Lock(stopChan)
		if err != nil {
			log.Printf("failed to contend for leadership. %s", err)
			time.Sleep(3 * time.Second)
			continue
		}

		if handle != nil {
			// We have the lock. We are the leader
			log.Printf("Manager %s has acquired leadership of the cluster", m.config.ID)

			m.SetRole(RoleLeader, m.config.ID)

			status := blockOnLock(handle, stopChan)
			log.Printf("Manager %s has lost leadership", m.config.ID)

			switch status {
			case lockBlockStatusStop:
				err := lock.Unlock()
				if err != nil {
					log.Printf("failed to release leadership lock. %s", err)
				}

			case lockBlockStatusContinue:
				log.Println("attempting to acquire leadership again")

			default:
				panic(fmt.Sprintf("unexpected lock status %d", status))
			}

			continue
		}

		// we don't have the lock. someone else has leadership
		pair, _, err := m.consul.KV().Get(m.lockKeyPath(), nil)
		if err != nil {
			log.Printf("failed to retrieve the ID of the new leader. %s", err)
			time.Sleep(3 * time.Second)
			continue
		}

		if pair == nil {
			log.Printf("lock key does not exist, starting leadership election again")
			time.Sleep(3 * time.Second)
			continue
		}

		m.SetRole(RoleFollower, string(pair.Value))
	}
}

func blockOnLock(handle <-chan struct{}, shutdown <-chan struct{}) lockBlockStatus {
	select {
	case <-handle:
		return lockBlockStatusContinue
	case <-shutdown:
		return lockBlockStatusStop
	}
}

func (m *Manager) createLeadershipLock() *api.Lock {
	opts := &api.LockOptions{
		Key:            m.lockKeyPath(),
		Value:          []byte(m.config.ID),
		MonitorRetries: 0,
		LockTryOnce:    true,
		LockWaitTime:   lockWaitTime,
		SessionOpts: &api.SessionEntry{
			Name:     "Tile38 cluster leadership manager session",
			TTL:      "15s",
			Behavior: api.SessionBehaviorDelete,
		},
	}

	lock, err := m.consul.LockOpts(opts)
	if err != nil {
		// LockOpts only returns an error when the key is missing
		// or session TTL is invalid. We ensure valid opts in argument
		// so this should never happen. Nevertheless, the error must
		// be dealt with as the lock is required for the leadership
		// mechanism to work, and there isn't anything we can do to fix
		// the problem at this point so a panic is the only way out.
		panic(fmt.Sprintf("failed to create LockOpts. %s", err))
	}

	return lock
}

func (m *Manager) lockKeyPath() string {
	return path.Join(m.config.ConsulLockPrefix, "leader")
}
