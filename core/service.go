package core

import (
	"errors"
	"fmt"
	"github.com/hashicorp/consul/api"
	"log"
)

func (m *Manager) serviceId(role Role) string {
	switch role {
	case RoleFollower:
		return fmt.Sprintf("%s-%s", m.config.FollowerServiceName, m.config.ID)
	case RoleLeader:
		return fmt.Sprintf("%s-%s", m.config.LeaderServiceName, m.config.ID)
	default:
		panic(fmt.Sprintf("unknown role %d", role))
	}
}

func (m *Manager) checkId(role Role) string {
	switch role {
	case RoleFollower:
		return fmt.Sprintf("%s-%s-health-check", m.config.FollowerServiceName, m.config.ID)
	case RoleLeader:
		return fmt.Sprintf("%s-%s-health-check", m.config.LeaderServiceName, m.config.ID)
	default:
		panic(fmt.Sprintf("unknown role %d", role))
	}
}

func (m *Manager) RegisterServices() error {
	log.Println("registering services in catalog")

	leader := &api.AgentServiceRegistration{
		ID:      m.serviceId(RoleLeader),
		Name:    m.config.LeaderServiceName,
		Port:    m.config.LeaderBindPort,
		Address: m.config.Advertise,
		Meta: map[string]string{
			"id": m.config.ID,
		},
		Connect: &api.AgentServiceConnect{
			Native: true,
		},
		Check: &api.AgentServiceCheck{
			CheckID:                        m.checkId(RoleLeader),
			Name:                           m.checkId(RoleLeader),
			Interval:                       "15s",
			Timeout:                        "3s",
			Status:                         api.HealthWarning,
			TCP:                            fmt.Sprintf("%s:%d", m.config.Advertise, m.config.LeaderBindPort),
			FailuresBeforeCritical:         3,
			DeregisterCriticalServiceAfter: "5m",
		},
	}

	opts := api.ServiceRegisterOpts{
		ReplaceExistingChecks: true,
	}

	err := m.consul.Agent().ServiceRegisterOpts(leader, opts)
	if err != nil {
		return err
	}

	follower := &api.AgentServiceRegistration{
		ID:      m.serviceId(RoleFollower),
		Name:    m.config.FollowerServiceName,
		Port:    m.config.FollowerBindPort,
		Address: m.config.Advertise,
		Meta: map[string]string{
			"id": m.config.ID,
		},
		Connect: &api.AgentServiceConnect{
			Native: true,
		},
		Check: &api.AgentServiceCheck{
			CheckID:                        m.checkId(RoleFollower),
			Name:                           m.checkId(RoleFollower),
			Interval:                       "15s",
			Timeout:                        "3s",
			Status:                         api.HealthWarning,
			TCP:                            fmt.Sprintf("%s:%d", m.config.Advertise, m.config.FollowerBindPort),
			FailuresBeforeCritical:         3,
			DeregisterCriticalServiceAfter: "5m",
		},
	}

	err = m.consul.Agent().ServiceRegisterOpts(follower, opts)
	if err != nil {
		return err
	}

	return nil
}

func (m *Manager) DeregisterServices() error {
	log.Println("deregistering catalog services")
	errL := m.consul.Agent().ServiceDeregister(m.serviceId(RoleLeader))
	errF := m.consul.Agent().ServiceDeregister(m.serviceId(RoleFollower))

	var e string
	for _, err := range []error{errL, errF} {
		if err != nil {
			e = fmt.Sprintf("%s\n", err.Error())
		}
	}

	if e != "" {
		return errors.New(e)
	}

	return nil
}
