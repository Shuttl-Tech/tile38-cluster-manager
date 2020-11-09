package core

import (
	"testing"
)

func TestPeer_Equal(t *testing.T) {
	cases := []struct {
		first  *Peer
		second *Peer
		result bool
	}{
		{
			first:  &Peer{},
			second: nil,
			result: false,
		},
		{
			first: &Peer{
				ID:      "one",
				Kind:    RoleLeader,
				Address: "first",
				Port:    4,
			},
			second: &Peer{
				ID:      "one",
				Kind:    RoleFollower,
				Address: "first",
				Port:    4,
			},
			result: false,
		},
		{
			first: &Peer{
				ID:      "one",
				Kind:    RoleLeader,
				Address: "first",
				Port:    4,
			},
			second: &Peer{
				ID:      "one",
				Kind:    RoleLeader,
				Address: "first",
				Port:    4,
			},
			result: true,
		},
	}

	for idx, test := range cases {
		result := test.first.Equal(test.second)
		if result != test.result {
			t.Errorf("case %d: unexpected result %v, expected %v", idx, result, test.result)
		}
	}
}

func TestPeers_Sort(t *testing.T) {
	cases := []struct {
		input  Peers
		expect Peers
	}{
		{
			input:  Peers{},
			expect: Peers{},
		},
		{
			input: Peers{
				{ID: "one"},
				{ID: "three"},
				{ID: "other"},
				{ID: "last"},
			},
			expect: Peers{
				{ID: "last"},
				{ID: "one"},
				{ID: "other"},
				{ID: "three"},
			},
		},
	}

	for idx, test := range cases {
		test.input.Sort()
		for x, peer := range test.input {
			if test.expect[x].ID != peer.ID {
				t.Errorf("case %d: item %d - %q != %q", idx, x, test.expect[x].ID, peer.ID)
			}
		}
	}
}

func TestPeers_Equal(t *testing.T) {
	cases := []struct {
		first  Peers
		second Peers
		expect bool
	}{
		{
			first: Peers{
				{ID: "one", Kind: RoleLeader, Address: "one", Port: 1},
				{ID: "two", Kind: RoleFollower, Address: "two", Port: 2},
				{ID: "three", Kind: RoleLeader, Address: "three", Port: 3},
			},
			second: Peers{
				{ID: "two", Kind: RoleFollower, Address: "two", Port: 2},
				{ID: "one", Kind: RoleLeader, Address: "one", Port: 1},
			},
			expect: false,
		},
		{
			first:  Peers{},
			second: Peers{},
			expect: true,
		},
		{
			first: Peers{
				{ID: "one", Kind: RoleLeader, Address: "one", Port: 1},
				{ID: "two", Kind: RoleFollower, Address: "two", Port: 2},
				{ID: "three", Kind: RoleLeader, Address: "three", Port: 3},
			},
			second: Peers{
				{ID: "two", Kind: RoleFollower, Address: "two", Port: 2},
				{ID: "three", Kind: RoleLeader, Address: "three", Port: 3},
				{ID: "one", Kind: RoleLeader, Address: "one", Port: 1},
			},
			expect: true,
		},
		{
			first: Peers{
				{ID: "one", Kind: RoleLeader, Address: "one", Port: 1},
				{ID: "two", Kind: RoleFollower, Address: "two", Port: 2},
				{ID: "three", Kind: RoleLeader, Address: "three", Port: 3},
			},
			second: Peers{
				{ID: "two", Kind: RoleFollower, Address: "two", Port: 2},
				{ID: "three", Kind: RoleLeader, Address: "three", Port: 3},
				{ID: "one", Kind: RoleLeader, Address: "one", Port: 1},
				{ID: "four", Kind: RoleLeader, Address: "four", Port: 4},
			},
			expect: false,
		},
	}

	for idx, test := range cases {
		result := test.first.Equal(test.second)
		if result != test.expect {
			t.Errorf("case %d: unexpected result %v, expected %v", idx, result, test.expect)
		}
	}
}
